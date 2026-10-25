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

// The extended suites run nightly rather than on every merge
const (
	ingressNginxManifest  = "https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.15.1/deploy/static/provider/kind/deploy.yaml"
	metricsServerManifest = "https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.9.0/components.yaml"
	logSenderTag          = "v2.2.1"
)

// As for the core suites, each suite is an ordered container, within a container that isn't, so a failing suite
// doesn't skip the others
var _ = Describe("Stroom operator, extended", Label("extended"), func() {
	Context("StroomCluster with node roles, HTTPS, an Ingress, the log sender and task autoscaling", Ordered,
		withOperator(featureSpecs))
	Context("StroomCluster volume claim delete policies", Ordered, withOperator(volumeClaimDeletePolicySpecs))
	Context("Operator restarts", Ordered, withOperator(operatorRestartSpecs))
	Context("Sample deployment with an operator-managed database", Ordered, withOperator(sampleOperatorDatabaseSpecs))
	Context("Sample deployment with an external database", Ordered, withOperator(sampleExternalDatabaseSpecs))
})

func featureSpecs() {
	const namespace = "stroom-e2e-features"
	const (
		processingPod = "stroom-e2e-node-processing-0"
		frontendPod   = "stroom-e2e-node-frontend-0"
	)
	values := keycloakClusterValues(operatorDatabase, namespace)
	values["STROOM_CONFIG_FILE"] = "external-idp-stroom-config-https.yaml"
	values["LOG_SENDER_TAG"] = logSenderTag

	BeforeAll(func() {
		installIngressNginx()
		installMetricsServer()
		deployKeycloakAndDatabase(operatorDatabase, namespace, values)
		for name, value := range generateTls(namespace) {
			values[name] = value
		}
		applyTestData(namespace, "tls-secrets.yaml", values)
		applyTestData(namespace, "stroom-cluster-extended.yaml", values)
	})

	It("should start each node set, after creating its key store", func() {
		for _, pod := range []string{processingPod, frontendPod} {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
			exitCode := mustKubectl("get", "pod", "-n", namespace, pod, "-o",
				`jsonpath={.status.initContainerStatuses[?(@.name=="generate-keystore")].state.terminated.exitCode}`)
			Expect(exitCode).To(Equal("0"), pod)
		}
	})

	It("should serve HTTPS", func() {
		Expect(httpStatus(namespace, processingPod, "https://localhost:8443/")).To(MatchRegexp(`^[1-5][0-9]{2}$`))
	})

	It("should enable all jobs on processing nodes, and the Reindex Content job on frontend nodes", func() {
		enabledJobs := func(pod string) string {
			return stroomDatabaseQuery(operatorDatabase, namespace, fmt.Sprintf(
				"SELECT j.name FROM stroom.job_node jn JOIN stroom.job j ON j.id = jn.job_id "+
					"WHERE jn.node_name = '%v' AND jn.enabled = 1 ORDER BY j.name", pod))
		}
		disabledJobCount := func(pod string) string {
			return strings.TrimSpace(stroomDatabaseQuery(operatorDatabase, namespace, fmt.Sprintf(
				"SELECT COUNT(*) FROM stroom.job_node WHERE node_name = '%v' AND enabled = 0", pod)))
		}
		Expect(disabledJobCount(processingPod)).To(Equal("0"))
		Expect(enabledJobs(frontendPod)).To(ContainSubstring("Reindex Content"))
		Expect(disabledJobCount(frontendPod)).ToNot(Equal("0"))
	})

	It("should route requests to Stroom through its Ingresses", func() {
		ingressIp := mustKubectl("get", "service", "-n", "ingress-nginx", "ingress-nginx-controller", "-o", "jsonpath={.spec.clusterIP}")
		viaIngress := func(method string, path string) string {
			return httpStatus(namespace, processingPod, "https://stroom.e2e.local"+path,
				"--resolve", "stroom.e2e.local:443:"+ingressIp, "-X", method)
		}
		// ingress-nginx can take a few seconds after a node is ready to route to it
		Eventually(func() string {
			return viaIngress("GET", "/")
		}, 2*time.Minute, 5*time.Second).Should(MatchRegexp(`^[23][0-9]{2}$`))

		// The datafeed Ingress reaches the processing nodes over HTTPS, so Stroom answers as it does directly. The
		// request has no feed, so Stroom rejects it, but not as the Ingress controller would (400 or 502).
		direct := httpStatus(namespace, processingPod, "https://localhost:8443/stroom/datafeed", "-X", "POST")
		Expect(direct).ToNot(BeElementOf("400", "502", "000"))
		Expect(viaIngress("POST", "/stroom/datafeed")).To(Equal(direct))
	})

	It("should send Stroom's logs with the log sender", func() {
		Eventually(func() string {
			output, _ := kubectl("logs", "-n", namespace, processingPod, "-c", "log-sender")
			return output
		}, 5*time.Minute, 15*time.Second).Should(ContainSubstring("STROOM-"))
	})

	It("should reduce the task limit of processing nodes when their CPU use is above the maximum", func() {
		taskLimit := func() string {
			return strings.TrimSpace(stroomDatabaseQuery(operatorDatabase, namespace, fmt.Sprintf(
				"SELECT jn.task_limit FROM stroom.job_node jn JOIN stroom.job j ON j.id = jn.job_id "+
					"WHERE jn.node_name = '%v' AND j.name = 'Data Processor'", processingPod)))
		}
		stroomDatabaseQuery(operatorDatabase, namespace, fmt.Sprintf(
			"UPDATE stroom.job_node jn JOIN stroom.job j ON j.id = jn.job_id SET jn.task_limit = 20 "+
				"WHERE jn.node_name = '%v' AND j.name = 'Data Processor'", processingPod))
		Expect(taskLimit()).To(Equal("20"))

		applyTestData(namespace, "task-autoscaler.yaml", values)
		Eventually(taskLimit, 10*time.Minute, 15*time.Second).ShouldNot(Equal("20"))
		Expect(operatorLogs()).To(ContainSubstring("Updating task limit for node '" + processingPod + "'"))
	})
}

func volumeClaimDeletePolicySpecs() {
	const namespace = "stroom-e2e-volumes"
	// Cluster name: volumeClaimDeletePolicy, or none
	policies := map[string]string{
		"keep":      "",
		"scaledown": "DeleteOnScaledownOnly",
		"always":    "DeleteOnScaledownAndClusterDeletion",
	}
	// The clusters share the DatabaseServer, so connect to it by address
	sharedDatabase := fmt.Sprintf(
		"{serverAddress: {host: stroom-e2e-db-headless.%v.svc.cluster.local, port: 3306, secretName: stroom-e2e-db}, userName: stroomuser}",
		namespace)
	pvc := func(cluster string, ordinal int) string {
		return fmt.Sprintf("pvc/data-stroom-%v-node-node-%v", cluster, ordinal)
	}
	expectPvc := func(cluster string, ordinal int, kept bool) {
		if kept {
			mustKubectl("get", pvc(cluster, ordinal), "-n", namespace)
		} else {
			waitForCondition(namespace, pvc(cluster, ordinal), "delete", defaultTimeout)
		}
	}

	BeforeAll(func() {
		deployPauseClusterPrerequisites(namespace)
		for cluster, policy := range policies {
			policyLine := "# No volumeClaimDeletePolicy"
			if policy != "" {
				policyLine = "volumeClaimDeletePolicy: " + policy
			}
			applyTestData(namespace, "stroom-cluster-pause.yaml", map[string]string{
				"CLUSTER_NAME": cluster, "VOLUME_CLAIM_DELETE_POLICY": policyLine, "NODE_COUNT": "2",
				"DATABASE_SERVER_REF": sharedDatabase, "HTTPS": noHttps, "LOG_SENDER": noLogSender,
			})
		}
	})

	It("should create a volume for each node", func() {
		for cluster := range policies {
			for ordinal := range 2 {
				waitForCondition(namespace, pvc(cluster, ordinal), "jsonpath={.status.phase}=Bound", defaultTimeout)
			}
		}
	})

	It("should delete the removed node's volume on scale-down, if the policy says to", func() {
		for cluster := range policies {
			mustKubectl("patch", "stroomcluster", cluster, "-n", namespace, "--type=json",
				"-p", `[{"op": "replace", "path": "/spec/nodeSets/0/count", "value": 1}]`)
		}
		for cluster := range policies {
			waitForCondition(namespace, fmt.Sprintf("pod/stroom-%v-node-node-1", cluster), "delete", defaultTimeout)
		}
		expectPvc("keep", 1, true)
		expectPvc("scaledown", 1, false)
		expectPvc("always", 1, false)
	})

	It("should delete the remaining volumes on cluster deletion, if the policy says to", func() {
		for cluster := range policies {
			mustKubectl("delete", "stroomcluster", cluster, "-n", namespace, "--wait=true", "--timeout=10m")
		}
		expectPvc("keep", 0, true)
		expectPvc("scaledown", 0, true)
		expectPvc("always", 0, false)
	})
}

func operatorRestartSpecs() {
	const namespace = "stroom-e2e-restart"
	const cluster = "restart"
	statefulSet := "statefulset/stroom-restart-node-node"
	pods := stroomPodNames(cluster, "node", 2)

	BeforeAll(func() {
		deployPauseClusterPrerequisites(namespace)
		applyTestData(namespace, "stroom-cluster-pause.yaml", map[string]string{
			"CLUSTER_NAME": cluster, "VOLUME_CLAIM_DELETE_POLICY": "# No volumeClaimDeletePolicy", "NODE_COUNT": "1",
			"DATABASE_SERVER_REF": "{serverRef: {name: e2e}}", "HTTPS": noHttps, "LOG_SENDER": noLogSender,
		})
		waitForCondition(namespace, "pod/"+pods[0], "jsonpath={.status.phase}=Running", defaultTimeout)
	})

	It("should leave the cluster unchanged when the operator restarts", func() {
		generation := mustKubectl("get", statefulSet, "-n", namespace, "-o", "jsonpath={.metadata.generation}")
		uids := podUids(namespace, pods[:1])

		restartOperator()
		Eventually(operatorLogs, 2*time.Minute, 5*time.Second).Should(ContainSubstring("StatefulSet reconciled"))
		Consistently(func() string {
			return mustKubectl("get", statefulSet, "-n", namespace, "-o", "jsonpath={.metadata.generation}")
		}, time.Minute, 10*time.Second).Should(Equal(generation))
		Expect(podUids(namespace, pods[:1])).To(Equal(uids))
	})

	It("should complete a scale-up when the operator restarts while applying it", func() {
		mustKubectl("patch", "stroomcluster", cluster, "-n", namespace, "--type=json",
			"-p", `[{"op": "replace", "path": "/spec/nodeSets/0/count", "value": 2}]`)
		restartOperator()
		waitForCondition(namespace, statefulSet, "jsonpath={.spec.replicas}=2", defaultTimeout)
		waitForCondition(namespace, "pod/"+pods[1], "jsonpath={.status.phase}=Running", defaultTimeout)
	})

	It("should keep waiting for nodes to drain when the operator restarts during cluster deletion", func() {
		By("registering a node with a processing task, as Stroom would")
		stroomDatabaseQuery(operatorDatabase, namespace, fmt.Sprintf("SET SESSION sql_mode = ''; SET FOREIGN_KEY_CHECKS = 0; "+
			"INSERT INTO stroom.node (name, enabled) VALUES ('%[1]v', 1); "+
			"INSERT INTO stroom.processor_task (fk_processor_node_id, status) SELECT id, %[2]v FROM stroom.node WHERE name = '%[1]v'",
			pods[0], processingTaskStatus))

		mustKubectl("delete", "stroomcluster", cluster, "-n", namespace, "--wait=false")
		waitingMessage := "StroomCluster deletion waiting on task completion for 1 nodes: " + pods[0]
		Eventually(operatorLogs, 2*time.Minute, 5*time.Second).Should(ContainSubstring(waitingMessage))

		restartOperator()
		Eventually(operatorLogs, 2*time.Minute, 5*time.Second).Should(ContainSubstring(waitingMessage))
		mustKubectl("get", "stroomcluster", cluster, "-n", namespace)

		By("completing the processing task")
		stroomDatabaseQuery(operatorDatabase, namespace, fmt.Sprintf(
			"DELETE FROM stroom.processor_task WHERE fk_processor_node_id = (SELECT id FROM stroom.node WHERE name = '%v')", pods[0]))
		waitForCondition(namespace, "stroomcluster/"+cluster, "delete", defaultTimeout)
	})
}

// deployPauseClusterPrerequisites creates a namespace with a DatabaseServer, and migrates the Stroom database, which
// the operator uses when deleting a StroomCluster. It uses the Stroom configuration bundled with the operator.
func deployPauseClusterPrerequisites(namespace string) {
	values := keycloakClusterValues(operatorDatabase, namespace)
	createNamespace(namespace)
	applyTestData(namespace, "secrets.yaml", values)
	deployDatabase(operatorDatabase, namespace, values)

	config, err := filepath.Abs(filepath.Join(projectDir, "internal", "controller", "static_content", "stroomcluster-config.yaml"))
	Expect(err).ToNot(HaveOccurred())
	mustKubectl("create", "configmap", "stroom-config", "-n", namespace, "--from-file=config.yml="+config)
	applyTestData(namespace, "provision-users.yaml", values)
	waitForCondition(namespace, "job/provision-users", "condition=Complete", stroomStartTimeout)
}

// installIngressNginx installs the ingress-nginx controller, if it isn't already
func installIngressNginx() {
	if _, err := kubectl("get", "deployment", "-n", "ingress-nginx", "ingress-nginx-controller"); err == nil {
		return
	}
	// The kind manifest schedules the controller on nodes with this label
	mustKubectl("label", "node", "--all", "ingress-ready=true", "--overwrite")
	mustKubectl("apply", "-f", ingressNginxManifest)
	waitForCondition("ingress-nginx", "deployment/ingress-nginx-controller", "condition=Available", defaultTimeout)
}

// installMetricsServer installs metrics-server, which task autoscaling requires, if it isn't already
func installMetricsServer() {
	if _, err := kubectl("get", "deployment", "-n", "kube-system", "metrics-server"); err == nil {
		return
	}
	mustKubectl("apply", "-f", metricsServerManifest)
	// kind's kubelet certificates aren't signed by the cluster CA
	mustKubectl("patch", "deployment", "metrics-server", "-n", "kube-system", "--type=json",
		"-p", `[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]`)
	waitForCondition("kube-system", "deployment/metrics-server", "condition=Available", defaultTimeout)
	Eventually(func() error {
		_, err := kubectl("top", "pods", "-n", "kube-system")
		return err
	}, 5*time.Minute, 10*time.Second).Should(Succeed())
}

// generateTls creates a CA and a certificate it signs, for the cluster's host name and Services, returning them as
// base64-encoded fixture values
func generateTls(namespace string) map[string]string {
	dir, err := filepath.Abs(GinkgoT().TempDir())
	Expect(err).ToNot(HaveOccurred())
	sans := fmt.Sprintf("DNS:stroom.e2e.local,DNS:localhost,DNS:*.%[1]v.svc.cluster.local,DNS:*.%[1]v.svc", namespace)
	script := fmt.Sprintf(`set -e
cd %[1]v
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj "/CN=stroom-e2e-ca" -keyout ca.key -out ca.crt
openssl req -newkey rsa:2048 -nodes -subj "/CN=stroom.e2e.local" -addext "subjectAltName=%[2]v" -keyout tls.key -out tls.csr
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 2 -copy_extensions copyall -out tls.crt
`, dir, sans)
	mustRun("sh", "-c", script)

	values := map[string]string{}
	for name, file := range map[string]string{"CA_CRT": "ca.crt", "TLS_CRT": "tls.crt", "TLS_KEY": "tls.key"} {
		content, err := os.ReadFile(filepath.Join(dir, file))
		Expect(err).ToNot(HaveOccurred())
		values[name] = base64.StdEncoding.EncodeToString(content)
	}
	return values
}

// httpStatus requests a URL from a Stroom node's container, returning the HTTP status code
func httpStatus(namespace string, pod string, url string, curlArgs ...string) string {
	command := "curl -sk --max-time 30 -o /dev/null -w '%{http_code}' " + strings.Join(curlArgs, " ") + " '" + url + "'"
	output, _ := execInPod(namespace, pod, "stroom-node", command)
	return strings.TrimSpace(output)
}
