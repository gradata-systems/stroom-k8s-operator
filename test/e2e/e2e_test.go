package e2e

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	defaultTimeout = 10 * time.Minute
	// stroomStartTimeout allows for the database migrations a new Stroom database needs
	stroomStartTimeout = 20 * time.Minute

	// Fixed values shared by the Keycloak realm, the Stroom configuration and the Secrets
	stroomClientSecret       = "e2e-stroom-client-secret"
	operatorClientSecret     = "e2e-operator-client-secret"
	stroomServiceAccountId   = "5b0c3c8e-6f6a-4f43-9a5e-2f1c7e3a9b01"
	operatorServiceAccountId = "a7d1f2c4-3e5b-4c6d-8e9f-0a1b2c3d4e5f"

	externalIdpCheck = "stroom.security.common.impl.ExternalIdpConfigurationProvider"

	// processingTaskStatus is the Stroom processor task status of a task being processed
	processingTaskStatus = 3

	// noLogSender is set on StroomClusters that don't use the log sender
	noLogSender = "# No logSender"
	// disabledLogSender is set on StroomClusters deployed by the previous release, which works around its bug where a
	// StroomCluster without a log sender is never deployed
	disabledLogSender = "logSender: {enabled: false, image: {repository: gchq/stroom-log-sender, tag: " + logSenderTag + "}}"

	// noHttps and withHttps are the HTTPS settings of a StroomCluster. withHttps needs the Secrets in tls-secrets.yaml.
	noHttps   = "# No HTTPS"
	withHttps = "https: {tlsSecretName: stroom-tls, tlsKeystorePasswordSecret: {secretName: stroom-keystore, key: password}}"

	// defaultSecurityContext and stroomUserSecurityContext are the pod security contexts of a node set. The Stroom
	// image starts as root by default, then runs Stroom as the stroom user, 1000.
	defaultSecurityContext    = "# Default pod security context"
	stroomUserSecurityContext = "podSecurityContext: {runAsNonRoot: true, runAsUser: 1000, runAsGroup: 1000, fsGroup: 1000}"
)

// The suites run in order, as each runs a Stroom cluster, and running them at once would need more memory than a
// typical workstation or CI runner has. The operator upgrade suite runs first, as it installs the previous release.
// Each other suite installs the current operator, if it isn't already. Each suite is an ordered container, so a
// failure skips its remaining specs, which depend on each other, while the enclosing container isn't, so the other
// suites still run. Ginkgo runs the suites in the order they're defined, as they're in the same top-level container.
var _ = Describe("Stroom operator", Label("core"), func() {
	Context("Operator upgrade", Ordered, operatorUpgradeSpecs)
	Context("Operator installed from the all-in-one manifest, moved to Helm", Ordered, manifestMigrationSpecs)
	Context("DatabaseServer", Ordered, withOperator(databaseServerSpecs))
	Context("StroomCluster with an external identity provider and an operator-managed database", Ordered,
		withOperator(externalIdpSpecs(operatorDatabase, "stroom-e2e-external")))
	Context("StroomCluster with an external identity provider and an external database", Ordered,
		withOperator(externalIdpSpecs(externalDatabase, "stroom-e2e-external-db")))
	Context("StroomCluster waiting on the Stroom bootstrap lock, and a Stroom upgrade", Ordered,
		withOperator(bootstrapLockSpecs))
	Context("StroomCluster with Stroom's internal identity provider", Ordered, withOperator(internalIdpSpecs))
})

// withOperator installs the current operator before a suite's specs, if it isn't already
func withOperator(specs func()) func() {
	return func() {
		BeforeAll(deployOperator)
		specs()
	}
}

func databaseServerSpecs() {
	const namespace = "stroom-e2e-db"
	servers := map[string][2]string{
		// DatabaseServer name: image repository and tag
		"oracle":   {"mysql/mysql-server", "8.0.32"},
		"official": {"mysql", "8.4"},
	}

	BeforeAll(func() {
		createNamespace(namespace)
		for name, image := range servers {
			applyTestData(namespace, "database-server.yaml", map[string]string{
				"DATABASE_SERVER_NAME": name, "MYSQL_REPOSITORY": image[0], "MYSQL_TAG": image[1],
			})
		}
	})

	It("should start each database server", func() {
		for name := range servers {
			waitForCondition(namespace, fmt.Sprintf("pod/stroom-%v-db-0", name), "condition=Ready", defaultTimeout)
			waitForMysql(namespace, name)
		}
	})

	It("should deploy a StroomCluster without a log sender, and delete it before any node has started", func() {
		applyTestData(namespace, "stroom-cluster-pause.yaml", map[string]string{
			"CLUSTER_NAME": "no-log-sender", "DATABASE_SERVER_REF": "{serverRef: {name: oracle}}", "NODE_COUNT": "1",
			"VOLUME_CLAIM_DELETE_POLICY": "# No volumeClaimDeletePolicy", "HTTPS": noHttps, "LOG_SENDER": noLogSender,
		})
		Eventually(func() error {
			_, err := kubectl("get", "statefulset", "-n", namespace, "stroom-no-log-sender-node-node")
			return err
		}, 2*time.Minute, 5*time.Second).Should(Succeed(), "the StroomCluster's StatefulSet should be created")
		Expect(mustKubectl("get", "stroomcluster", "no-log-sender", "-n", namespace, "-o", "jsonpath={.metadata.finalizers}")).
			ToNot(BeEmpty())

		// The database has no Stroom tables, as Stroom has never run, so there's nothing to drain
		mustKubectl("delete", "stroomcluster", "no-log-sender", "-n", namespace, "--wait=true", "--timeout=2m")
		Expect(operatorLogs()).To(ContainSubstring("The Stroom database has no job_node table"))
	})

	It("should create the Stroom databases", func() {
		for name := range servers {
			output := mysqlAsRoot(namespace, name, rootPassword(namespace, name), "SHOW DATABASES")
			Expect(output).To(ContainSubstring("stroom"))
			Expect(output).To(ContainSubstring("stats"))
		}
	})

	It("should set the root password from the Secret, with each image", func() {
		for name := range servers {
			Expect(mysqlAsRoot(namespace, name, rootPassword(namespace, name), "SELECT 'ok'")).To(ContainSubstring("ok"), name)
			_, err := execInPod(namespace, fmt.Sprintf("stroom-%v-db-0", name), "mysql",
				"MYSQL_PWD='/etc/mysql/password/root' mysql -uroot -N -e \"SELECT 'ok'\"")
			Expect(err).To(HaveOccurred(), "%v: the password file's path should not be the password", name)
		}
	})

	It("should back up the databases on the schedule's CronJob", func() {
		// The backup runs in the official image, as the mysql/mysql-server image has no gzip, and its mysqldump
		// needs a privilege the database user doesn't have. The official image's mysqldump can back up 8.0 servers.
		applyTestData(namespace, "database-backup.yaml", map[string]string{
			"DATABASE_SERVER_NAME": "oracle", "MYSQL_REPOSITORY": "mysql", "MYSQL_TAG": "8.4",
		})
		Eventually(func() error {
			_, err := kubectl("get", "cronjob", "-n", namespace, "stroom-oracle-db-backup")
			return err
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("running a backup now, as the docs describe")
		mustKubectl("create", "job", "-n", namespace, "--from=cronjob/stroom-oracle-db-backup", "e2e-backup")
		waitForCondition(namespace, "job/e2e-backup", "condition=Complete", defaultTimeout)
		output := mustKubectl("logs", "-n", namespace, "job/e2e-backup")
		Expect(output).To(ContainSubstring("Backing up databases (stroom,stats) to: /var/lib/mysql/backup/"))
		Expect(output).To(ContainSubstring("Backup successful"))
	})
}

// databaseSource is where a StroomCluster's database server comes from
type databaseSource int

const (
	// operatorDatabase is a DatabaseServer managed by the operator
	operatorDatabase databaseSource = iota
	// externalDatabase is a MySQL server the operator doesn't manage, which the StroomCluster connects to by address
	externalDatabase
)

// externalIdpSpecs tests a StroomCluster using Keycloak, with a database server from the given source. The identity
// provider outage is only tested with an operator-managed database, as it doesn't depend on the database.
func externalIdpSpecs(database databaseSource, namespace string) func() {
	return func() {
		pods := stroomPodNames("e2e", "node", stroomNodeCount())
		values := keycloakClusterValues(database, namespace)

		BeforeAll(func() {
			deployKeycloakAndDatabase(database, namespace, values)
			applyTestData(namespace, "stroom-cluster.yaml", values)
		})

		stroomClusterSpecs(namespace, pods)

		if database == externalDatabase {
			It("should keep Stroom's data in the external database", func() {
				output := mustKubectl("exec", "-n", namespace, "mysql-0", "-c", "mysql", "--", "sh", "-c",
					`MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -N -e "SELECT name FROM stroom.node ORDER BY name"`)
				for _, pod := range pods {
					Expect(output).To(ContainSubstring(pod))
				}
			})
		}

		if database == operatorDatabase {
			scalingSpecs(namespace, pods)
			identityProviderOutageSpecs(namespace, pods)
		}

		// Deletes the StroomCluster, so must be last
		clusterDeletionSpecs(database, namespace, pods)
	}
}

// keycloakClusterValues returns the fixture values for a StroomCluster using Keycloak, which can be overridden
func keycloakClusterValues(database databaseSource, namespace string) map[string]string {
	values := map[string]string{
		"NAMESPACE":                   namespace,
		"STROOM_TAG":                  stroomTag,
		"PROVISIONING_STROOM_TAG":     stroomTag,
		"KEYCLOAK_IMAGE":              keycloakImage,
		"STROOM_CLIENT_SECRET":        stroomClientSecret,
		"OPERATOR_CLIENT_SECRET":      operatorClientSecret,
		"STROOM_SERVICE_ACCOUNT_ID":   stroomServiceAccountId,
		"OPERATOR_SERVICE_ACCOUNT_ID": operatorServiceAccountId,
		"CONFIG_MAP_REF":              "configMapRef: {name: stroom-config, itemName: config.yml}",
		"STROOM_CONFIG_FILE":          "external-idp-stroom-config.yaml",
		"HTTPS":                       noHttps,
		"POD_SECURITY_CONTEXT":        defaultSecurityContext,
		"STROOM_NODES":                stroomNodes,
		"LOG_SENDER":                  noLogSender,
		"STARTUP_INITIAL_DELAY":       "10",
		"STARTUP_FAILURE_THRESHOLD":   "60",
		"LOCK_WAIT_ALLOWANCE":         "# Default lockWaitAllowanceSeconds",
	}
	for name, value := range databaseValues(database, namespace) {
		values[name] = value
	}
	return values
}

// deployKeycloakAndDatabase creates a namespace with Keycloak, the Stroom configuration and a database server, then
// creates the Stroom user for the operator, which also migrates the Stroom database
func deployKeycloakAndDatabase(database databaseSource, namespace string, values map[string]string) {
	createNamespace(namespace)
	applyTestData(namespace, "secrets.yaml", values)
	applyTestData(namespace, "keycloak.yaml", values)
	applyStroomConfig(namespace, values["STROOM_CONFIG_FILE"])
	deployDatabase(database, namespace, values)
	waitForCondition(namespace, "deployment/keycloak", "condition=Available", defaultTimeout)

	By("creating the Stroom user for the operator, and migrating the database")
	applyTestData(namespace, "provision-users.yaml", values)
	waitForCondition(namespace, "job/provision-users", "condition=Complete", stroomStartTimeout)
}

// databaseValues returns the fixture values that connect a StroomCluster to its database server
func databaseValues(database databaseSource, namespace string) map[string]string {
	if database == externalDatabase {
		host := fmt.Sprintf("mysql.%v.svc.cluster.local", namespace)
		return map[string]string{
			"DATABASE_SERVER_REF":          fmt.Sprintf("{serverAddress: {host: %v, port: 3306, secretName: external-mysql}, userName: stroomuser}", host),
			"DB_HOST":                      host,
			"DB_SECRET_NAME":               "external-mysql",
			"EXTERNAL_MYSQL_ROOT_PASSWORD": "e2e-external-root-password",
			"EXTERNAL_MYSQL_PASSWORD":      "e2e-external-stroomuser-password",
		}
	}
	return map[string]string{
		"DATABASE_SERVER_REF":  "{serverRef: {name: e2e}}",
		"DATABASE_SERVER_NAME": "e2e",
		"MYSQL_REPOSITORY":     "mysql/mysql-server",
		"MYSQL_TAG":            "8.0.32",
		"DB_HOST":              fmt.Sprintf("stroom-e2e-db-headless.%v.svc.cluster.local", namespace),
		"DB_SECRET_NAME":       "stroom-e2e-db",
	}
}

// deployDatabase deploys a database server, and waits for it to accept connections
func deployDatabase(database databaseSource, namespace string, values map[string]string) {
	if database == externalDatabase {
		applyTestData(namespace, "external-mysql.yaml", values)
		waitForCondition(namespace, "pod/mysql-0", "condition=Ready", defaultTimeout)
		return
	}
	applyTestData(namespace, "database-server.yaml", values)
	waitForCondition(namespace, "pod/stroom-e2e-db-0", "condition=Ready", defaultTimeout)
	waitForMysql(namespace, "e2e")
}

// applyStroomConfig applies a Stroom configuration for Keycloak in the namespace. It contains Stroom's own ${VAR}
// references, so only the namespace in the Keycloak address is substituted.
func applyStroomConfig(namespace string, file string) {
	content, err := os.ReadFile(filepath.Join("testdata", file))
	Expect(err).ToNot(HaveOccurred())
	config := strings.ReplaceAll(string(content), "keycloak.stroom-e2e-external.svc", "keycloak."+namespace+".svc")
	path, err := filepath.Abs(filepath.Join(GinkgoT().TempDir(), "stroom-config.yaml"))
	Expect(err).ToNot(HaveOccurred())
	Expect(os.WriteFile(path, []byte(config), 0o600)).To(Succeed())
	mustKubectl("apply", "-n", namespace, "-f", path)
}

// stroomClusterSpecs tests a running StroomCluster whose nodes can authenticate to Stroom
func stroomClusterSpecs(namespace string, pods []string) {
	It("should start every node", func() {
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
		}
	})

	It("should enable each node and its jobs on startup", func() {
		for _, pod := range pods {
			output, err := execInPod(namespace, pod, "stroom-node", "cat /stroom/logs/k8s/node-start.log")
			Expect(err).ToNot(HaveOccurred())
			Expect(output).To(ContainSubstring("jobs enabled"))
		}
	})

	It("should probe liveness with only the deadlocks check, and readiness without the identity provider check", func() {
		probePath := func(probe string) string {
			return mustKubectl("get", "statefulset", "-n", namespace, "stroom-e2e-node-node", "-o",
				fmt.Sprintf(`jsonpath={.spec.template.spec.containers[?(@.name=="stroom-node")].%v.httpGet.path}`, probe))
		}
		Expect(probePath("livenessProbe")).To(Equal("/stroomAdmin/filteredhealthcheck?allow=deadlocks"))
		Expect(probePath("readinessProbe")).To(Equal("/stroomAdmin/filteredhealthcheck?deny=" + externalIdpCheck))
	})

	It("should report health for the checks a probe filter would name", func() {
		statusCode := func(query string) string {
			output, err := execInPod(namespace, pods[0], "stroom-node",
				"curl -s --max-time 30 -o /dev/null -w '%{http_code}' 'http://localhost:8081/stroomAdmin/filteredhealthcheck"+query+"'")
			Expect(err).ToNot(HaveOccurred())
			return output
		}
		Expect(statusCode("")).To(Equal("200"))
		Expect(statusCode("?allow=deadlocks")).To(Equal("200"))
		Expect(statusCode("?deny=" + externalIdpCheck)).To(Equal("200"))
		Expect(statusCode("?allow=" + externalIdpCheck)).To(Equal("200"))
		// An unknown health check name is an error, which fails the probe
		Expect(statusCode("?allow=unknown")).To(Equal("500"))
	})

	It("should drain a node before it stops", func() {
		pod := pods[len(pods)-1]
		mustKubectl("delete", "pod", "-n", namespace, pod, "--wait=true", "--timeout=5m")
		waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)

		// Logs are kept on the node's volume, so the replacement pod has the previous pod's pre-stop log
		output, err := execInPod(namespace, pod, "stroom-node", "cat /stroom/logs/k8s/pre-stop.log")
		Expect(err).ToNot(HaveOccurred())
		Expect(output).To(ContainSubstring("All tasks drained"))
	})
}

// scalingSpecs tests scaling a StroomCluster's node set up by one node, then back down. The cluster's database is a
// DatabaseServer named e2e.
func scalingSpecs(namespace string, pods []string) {
	addedPod := stroomPodNames("e2e", "node", len(pods)+1)[len(pods)]

	scaleTo := func(count int) {
		mustKubectl("patch", "stroomcluster", "e2e", "-n", namespace, "--type=json",
			"-p", fmt.Sprintf(`[{"op": "replace", "path": "/spec/nodeSets/0/count", "value": %v}]`, count))
	}
	podUid := func(pod string) string {
		return mustKubectl("get", "pod", "-n", namespace, pod, "-o", "jsonpath={.metadata.uid}")
	}
	startupFailureThreshold := func() string {
		return mustKubectl("get", "statefulset", "-n", namespace, "stroom-e2e-node-node", "-o",
			`jsonpath={.spec.template.spec.containers[?(@.name=="stroom-node")].startupProbe.failureThreshold}`)
	}

	// Under the Parallel pod management policy, the startup probe allows the default 30 seconds, 3 more failures, for
	// each node after the first, so scaling changes the pod template, and rolls the existing nodes
	startupFailureThresholdFor := func(nodes int) string {
		return fmt.Sprint(60 + 3*(nodes-1))
	}
	rollout := func() {
		mustKubectl("rollout", "status", "statefulset/stroom-e2e-node-node", "-n", namespace, "--timeout=30m")
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
		}
	}

	It("should start a node when the node set is scaled up, and roll the existing nodes with a longer startup", func() {
		Expect(startupFailureThreshold()).To(Equal(startupFailureThresholdFor(len(pods))))
		uids := map[string]string{}
		for _, pod := range pods {
			uids[pod] = podUid(pod)
		}

		scaleTo(len(pods) + 1)
		waitForCondition(namespace, "pod/"+addedPod, "condition=Ready", stroomStartTimeout)
		output, err := execInPod(namespace, addedPod, "stroom-node", "cat /stroom/logs/k8s/node-start.log")
		Expect(err).ToNot(HaveOccurred())
		Expect(output).To(ContainSubstring("jobs enabled"))

		Expect(startupFailureThreshold()).To(Equal(startupFailureThresholdFor(len(pods) + 1)))
		rollout()
		for _, pod := range pods {
			Expect(podUid(pod)).ToNot(Equal(uids[pod]), "%v should have been replaced", pod)
		}
	})

	It("should drain and remove the node when the node set is scaled down, keeping its volume", func() {
		scaleTo(len(pods))
		waitForCondition(namespace, "pod/"+addedPod, "delete", defaultTimeout)
		Expect(startupFailureThreshold()).To(Equal(startupFailureThresholdFor(len(pods))))
		rollout()

		// The pre-stop hook disables the node in Stroom before it stops
		enabled := mysqlAsRoot(namespace, "e2e", rootPassword(namespace, "e2e"),
			fmt.Sprintf("SELECT enabled FROM stroom.node WHERE name = '%v'", addedPod))
		Expect(strings.TrimSpace(enabled)).To(Equal("0"))

		// With no volumeClaimDeletePolicy, the node's PersistentVolumeClaim is kept
		mustKubectl("get", "pvc", "-n", namespace, "data-"+addedPod)
	})
}

// clusterDeletionSpecs tests deleting a StroomCluster, which the operator holds back until the nodes have drained
func clusterDeletionSpecs(database databaseSource, namespace string, pods []string) {
	// A node of another StroomCluster sharing the database, whose name starts with this cluster's resource name
	const otherClusterNode = "stroom-e2e2-node-node-0"

	enabledJobCount := func(nodeName string) string {
		return strings.TrimSpace(stroomDatabaseQuery(database, namespace,
			fmt.Sprintf("SELECT COUNT(*) FROM stroom.job_node WHERE node_name = '%v' AND enabled = 1", nodeName)))
	}

	It("should hold the cluster and its database server with finalizers", func() {
		Expect(mustKubectl("get", "stroomcluster", "e2e", "-n", namespace, "-o", "jsonpath={.metadata.finalizers}")).
			To(ContainSubstring("waitnodetasks.finalizers.stroom.gchq.github.io"))
		if database == operatorDatabase {
			Expect(mustKubectl("get", "databaseserver", "e2e", "-n", namespace, "-o", "jsonpath={.metadata.finalizers}")).
				To(ContainSubstring("stroomcluster.finalizers.stroom.gchq.github.io"))
		}
	})

	It("should disable the nodes' jobs and remove the cluster when the StroomCluster is deleted", func() {
		By("copying a node's jobs to a node of another cluster")
		columns := strings.TrimSpace(stroomDatabaseQuery(database, namespace,
			"SELECT GROUP_CONCAT(column_name ORDER BY ordinal_position) FROM information_schema.columns "+
				"WHERE table_schema = 'stroom' AND table_name = 'job_node' AND column_name NOT IN ('id', 'node_name')"))
		stroomDatabaseQuery(database, namespace, fmt.Sprintf(
			"INSERT INTO stroom.job_node (node_name, %[1]v) SELECT '%[2]v', %[1]v FROM stroom.job_node WHERE node_name = '%[3]v'",
			columns, otherClusterNode, pods[0]))
		stroomDatabaseQuery(database, namespace, fmt.Sprintf(
			"UPDATE stroom.job_node SET enabled = 1 WHERE node_name = '%v'", otherClusterNode))
		for _, pod := range append([]string{otherClusterNode}, pods...) {
			Expect(enabledJobCount(pod)).ToNot(Equal("0"), pod)
		}

		By("adding a processing task for a node, as though it were still processing data")
		// Other columns are left to their implicit defaults, and foreign keys unchecked, as only the node and
		// status are used to determine whether the node has drained
		stroomDatabaseQuery(database, namespace, fmt.Sprintf("SET SESSION sql_mode = ''; SET FOREIGN_KEY_CHECKS = 0; "+
			"INSERT INTO stroom.processor_task (fk_processor_node_id, status) "+
			"SELECT id, %v FROM stroom.node WHERE name = '%v'", processingTaskStatus, pods[0]))

		mustKubectl("delete", "stroomcluster", "e2e", "-n", namespace, "--wait=false")
		Eventually(operatorLogs, 2*time.Minute, 5*time.Second).
			Should(ContainSubstring("StroomCluster deletion waiting on task completion for 1 nodes: " + pods[0]))
		Consistently(func() error {
			_, err := kubectl("get", "stroomcluster", "e2e", "-n", namespace)
			return err
		}, 30*time.Second, 5*time.Second).Should(Succeed(), "the StroomCluster should be kept while a node has processing tasks")

		By("completing the processing task")
		stroomDatabaseQuery(database, namespace, fmt.Sprintf(
			"DELETE FROM stroom.processor_task WHERE status = %v AND fk_processor_node_id = (SELECT id FROM stroom.node WHERE name = '%v')",
			processingTaskStatus, pods[0]))
		waitForCondition(namespace, "stroomcluster/e2e", "delete", defaultTimeout)

		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "delete", defaultTimeout)
			Expect(enabledJobCount(pod)).To(Equal("0"), pod)
		}
		Expect(operatorLogs()).
			To(ContainSubstring("All tasks drained, deletion commencing"))

		// The other cluster's node is left alone, although its name starts with this cluster's resource name
		Expect(enabledJobCount(otherClusterNode)).ToNot(Equal("0"))
	})

	It("should remove the cluster's Ingresses, and keep its volumes", func() {
		Expect(mustKubectl("get", "ingress", "-n", namespace, "-l", "stroom/cluster=e2e", "-o", "name")).To(BeEmpty())
		// With no volumeClaimDeletePolicy, the nodes' PersistentVolumeClaims are kept
		for _, pod := range pods {
			mustKubectl("get", "pvc", "-n", namespace, "data-"+pod)
		}
	})

	if database == operatorDatabase {
		It("should release the database server", func() {
			Eventually(func() string {
				return mustKubectl("get", "databaseserver", "e2e", "-n", namespace, "-o", "jsonpath={.metadata.finalizers}")
			}, time.Minute, 5*time.Second).ShouldNot(ContainSubstring("stroomcluster.finalizers.stroom.gchq.github.io"))
		})
	}
}

// stroomDatabaseQuery runs a statement against the Stroom cluster's database server, as the MySQL root user
func stroomDatabaseQuery(database databaseSource, namespace string, statement string) string {
	if database == externalDatabase {
		var output string
		EventuallyWithOffset(1, func() error {
			var err error
			output, err = kubectl("exec", "-n", namespace, "mysql-0", "-c", "mysql", "--", "sh", "-c",
				fmt.Sprintf(`MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -N -e "%v"`, statement))
			return err
		}, 5*time.Minute, 5*time.Second).Should(Succeed())
		return output
	}
	return mysqlAsRoot(namespace, "e2e", rootPassword(namespace, "e2e"), statement)
}

// identityProviderOutageSpecs tests how a running StroomCluster behaves while Keycloak is unavailable
func identityProviderOutageSpecs(namespace string, pods []string) {
	It("should keep every node running and ready during an identity provider outage", func() {
		// The liveness probe only runs the deadlocks check, and the readiness probe skips the identity provider's
		restarts := map[string]int{}
		for _, pod := range pods {
			restarts[pod] = podRestartCount(namespace, pod)
		}

		By("stopping Keycloak")
		mustKubectl("scale", "deployment/keycloak", "-n", namespace, "--replicas=0")
		Eventually(func() string {
			// The identity provider's health check reports the outage, or doesn't answer within a probe's timeout, so
			// the probes would fail without their filters. curl gives up after 10 seconds, reporting 000.
			output, _ := execInPod(namespace, pods[0], "stroom-node",
				"curl -s --max-time 10 -o /dev/null -w '%{http_code}' 'http://localhost:8081/stroomAdmin/filteredhealthcheck?allow="+externalIdpCheck+"'")
			return output
		}, 5*time.Minute, 15*time.Second).ShouldNot(Equal("200"))
		Consistently(func(g Gomega) {
			for _, pod := range pods {
				g.Expect(podRestartCount(namespace, pod)).To(Equal(restarts[pod]), pod)
				g.Expect(mustKubectl("get", "pod", "-n", namespace, pod, "-o",
					`jsonpath={.status.conditions[?(@.type=="Ready")].status}`)).To(Equal("True"), pod)
			}
		}, 3*time.Minute, 15*time.Second).Should(Succeed())

		By("restarting Keycloak")
		mustKubectl("scale", "deployment/keycloak", "-n", namespace, "--replicas=1")
		waitForCondition(namespace, "deployment/keycloak", "condition=Available", defaultTimeout)
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
		}
	})
}

func internalIdpSpecs() {
	const namespace = "stroom-e2e-internal"
	pod := stroomPodNames("e2e", "node", 1)[0]
	values := map[string]string{
		"NAMESPACE":              namespace,
		"STROOM_TAG":             stroomTag,
		"STROOM_CLIENT_SECRET":   stroomClientSecret,
		"OPERATOR_CLIENT_SECRET": operatorClientSecret,
		"CONFIG_MAP_REF":         "# Uses the Stroom configuration bundled with the operator",
		"HTTPS":                  noHttps,
		"POD_SECURITY_CONTEXT":   defaultSecurityContext,
		"STROOM_NODES":           "1",
		"LOG_SENDER":             noLogSender,
		"STARTUP_INITIAL_DELAY":  "10",
		// Allow time for the initial database migration, then give up
		"STARTUP_FAILURE_THRESHOLD": "90",
		"LOCK_WAIT_ALLOWANCE":       "# Default lockWaitAllowanceSeconds",
	}

	for name, value := range databaseValues(operatorDatabase, namespace) {
		values[name] = value
	}

	BeforeAll(func() {
		createNamespace(namespace)
		applyTestData(namespace, "secrets.yaml", values)
		deployDatabase(operatorDatabase, namespace, values)
	})

	// waitForStroom waits for the node's Stroom to report healthy. It never becomes ready, as its startup probe fails.
	waitForStroom := func() {
		Eventually(func() string {
			output, _ := execInPod(namespace, pod, "stroom-node",
				"curl -s --max-time 30 -o /dev/null -w '%{http_code}' http://localhost:8081/stroomAdmin/filteredhealthcheck")
			return output
		}, stroomStartTimeout, 15*time.Second).Should(Equal("200"))
	}

	It("should start Stroom without HTTPS, with the bundled configuration", func() {
		applyTestData(namespace, "stroom-cluster.yaml", values)
		waitForStroom()
		Expect(podRestartCount(namespace, pod)).To(Equal(0))
	})

	It("should start Stroom with HTTPS, with the key store readable by the stroom user", func() {
		for name, value := range generateTls(namespace) {
			values[name] = value
		}
		values["HTTPS"] = withHttps
		applyTestData(namespace, "tls-secrets.yaml", values)
		applyTestData(namespace, "stroom-cluster.yaml", values)
		replaceNodePod(namespace, pod, "{.spec.template.spec.initContainers[*].name}", "generate-keystore")

		waitForStroom()
		Expect(httpStatus(namespace, pod, "https://localhost:8443/")).To(MatchRegexp(`^[1-5][0-9]{2}$`))
		owner, err := execInPod(namespace, pod, "stroom-node", "stat -c '%U %a' /stroom/pki/tls/keystore.p12")
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.TrimSpace(owner)).To(Equal("stroom 400"))
	})

	It("should keep running Stroom when the node set is changed to run as the stroom user", func() {
		// The lifecycle scripts have been running as root, and give their logs to the stroom user
		values["POD_SECURITY_CONTEXT"] = stroomUserSecurityContext
		applyTestData(namespace, "stroom-cluster.yaml", values)
		replaceNodePod(namespace, pod, "{.spec.template.spec.securityContext.runAsUser}", "1000")

		waitForStroom()
		Expect(httpStatus(namespace, pod, "https://localhost:8443/")).To(MatchRegexp(`^[1-5][0-9]{2}$`))
		user, err := execInPod(namespace, pod, "stroom-node", "id -u")
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.TrimSpace(user)).To(Equal("1000"))
		_, err = execInPod(namespace, pod, "stroom-node", "test -w /stroom/logs/k8s/api.log")
		Expect(err).ToNot(HaveOccurred(), "the lifecycle scripts' log should be writable by the stroom user")
	})

	It("should fail to obtain a token for the node startup script", func() {
		// BASELINE: known issue. Stroom 7.13 rejects client credentials token requests with its internal identity
		// provider, so the node lifecycle scripts can't authenticate. The script also logs a failure, with response
		// code 000, while Stroom is starting, so this looks for Stroom's rejection, a 400 Bad Request.
		Eventually(func(g Gomega) {
			output, err := execInPod(namespace, pod, "stroom-node", "cat /stroom/logs/k8s/api.log")
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(output).To(ContainSubstring("fetchClientCredsToken failed. Response code: 400"))
		}, defaultTimeout, 15*time.Second).Should(Succeed())
	})

	It("should never become ready", func() {
		// BASELINE: known issue
		ready := mustKubectl("get", "pod", "-n", namespace, pod, "-o", `jsonpath={.status.conditions[?(@.type=="Ready")].status}`)
		Expect(ready).To(Equal("False"))
	})
}

// rootPassword returns the generated MySQL root password for a DatabaseServer
func rootPassword(namespace string, dbServer string) string {
	encoded := mustKubectl("get", "secret", "-n", namespace, fmt.Sprintf("stroom-%v-db", dbServer), "-o", "jsonpath={.data.root}")
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return string(decoded)
}

// mysqlAsRoot runs a statement as the MySQL root user, failing the spec if it can't log in. MySQL may not be
// accepting connections yet, even though its pod is ready, so it retries for a while.
func mysqlAsRoot(namespace string, dbServer string, password string, statement string) string {
	var output string
	EventuallyWithOffset(1, func() error {
		var err error
		output, err = execInPod(namespace, fmt.Sprintf("stroom-%v-db-0", dbServer), "mysql",
			fmt.Sprintf("MYSQL_PWD='%v' mysql -uroot -N -e \"%v\"", password, statement))
		return err
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
	return output
}

// waitForMysql waits until MySQL accepts connections, which can be some time after its pod is ready
func waitForMysql(namespace string, dbServer string) {
	EventuallyWithOffset(1, func() error {
		_, err := execInPod(namespace, fmt.Sprintf("stroom-%v-db-0", dbServer), "mysql",
			"mysqladmin -u healthcheck ping && mysql -u healthcheck -N -e 'SELECT 1'")
		return err
	}, defaultTimeout, 5*time.Second).Should(Succeed())
}
