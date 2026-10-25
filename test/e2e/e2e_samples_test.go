package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The samples are deployed as users would, with kubectl apply -k, through overlays in testdata/samples that size them
// for a kind cluster. Keycloak and, for external-database, MySQL are deployed alongside, from the test fixtures.

// sampleOperatorDatabaseSpecs tests the operator-database sample, with its NetworkPolicies in force
func sampleOperatorDatabaseSpecs() {
	const namespace = "stroom-e2e-samples"
	pods := []string{"stroom-dev-node-data-0", "stroom-dev-node-ui-0"}
	values := sampleValues(namespace)
	values["DB_HOST"] = "stroom-dev-db-headless." + namespace + ".svc.cluster.local"
	values["DB_SECRET_NAME"] = "stroom-dev-db"
	var cluster string

	BeforeAll(func() {
		installIngressNginx()
		installMetricsServer()
		cluster = deploySamplePrerequisites(namespace, "operator-database", values)
		waitForCondition(namespace, "pod/stroom-dev-db-0", "condition=Ready", defaultTimeout)
		waitForMysql(namespace, "dev")
		provisionSampleUsers(namespace, values)
	})

	It("should start the sample's Stroom cluster", func() {
		applyManifest(namespace, cluster)
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
			Expect(podRestartCount(namespace, pod)).To(Equal(0), "%v should start within the default startup probe", pod)
		}
	})

	It("should serve Stroom through its Ingress, with HTTPS", func() {
		expectServedViaIngress(namespace, pods[0])
	})

	It("should back up the databases", func() {
		mustKubectl("create", "job", "-n", namespace, "--from=cronjob/stroom-dev-db-backup", "e2e-backup")
		waitForCondition(namespace, "job/e2e-backup", "condition=Complete", defaultTimeout)
		Expect(mustKubectl("logs", "-n", namespace, "job/e2e-backup")).To(ContainSubstring("Backup successful"))
	})

	It("should refuse connections the NetworkPolicies don't allow", func() {
		// From a pod outside the namespace, which isn't the Ingress controller or the operator
		reachable := func(host string, port int) string {
			pod := fmt.Sprintf("e2e-network-check-%v", port)
			_, _ = kubectl("delete", "pod", "-n", "default", pod, "--ignore-not-found", "--wait=true")
			mustKubectl("run", pod, "-n", "default", "--image=mysql:8.4", "--image-pull-policy=IfNotPresent",
				"--restart=Never", "--command", "--", "bash", "-c",
				fmt.Sprintf("if timeout 5 bash -c '< /dev/tcp/%v/%v'; then echo open; else echo blocked; fi", host, port))
			waitForCondition("default", "pod/"+pod, "jsonpath={.status.phase}=Succeeded", 2*time.Minute)
			output := strings.TrimSpace(mustKubectl("logs", "-n", "default", pod))
			mustKubectl("delete", "pod", "-n", "default", pod, "--wait=false")
			return output
		}
		Expect(reachable(values["DB_HOST"], 3306)).To(Equal("blocked"))
		Expect(reachable("stroom-dev-node-data-http."+namespace+".svc.cluster.local", 8080)).To(Equal("blocked"))
	})
}

// sampleExternalDatabaseSpecs tests the external-database sample, with a MySQL server deployed from a test fixture
func sampleExternalDatabaseSpecs() {
	const namespace = "stroom-e2e-samples-external"
	pod := "stroom-dev-node-node-0"
	values := sampleValues(namespace)
	for name, value := range databaseValues(externalDatabase, namespace) {
		values[name] = value
	}
	// The sample's Secret holds the stroomuser password
	values["DB_SECRET_NAME"] = "stroom-db"
	var cluster string

	BeforeAll(func() {
		installIngressNginx()
		cluster = deploySamplePrerequisites(namespace, "external-database", values)
		deployDatabase(externalDatabase, namespace, values)
		provisionSampleUsers(namespace, values)
	})

	It("should start the sample's Stroom cluster", func() {
		applyManifest(namespace, cluster)
		waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
		Expect(podRestartCount(namespace, pod)).To(Equal(0), "the node should start within the default startup probe")
	})

	It("should keep Stroom's data in the external database", func() {
		output := mustKubectl("exec", "-n", namespace, "mysql-0", "-c", "mysql", "--", "sh", "-c",
			`MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -N -e "SELECT name FROM stroom.node"`)
		Expect(output).To(ContainSubstring(pod))
	})

	It("should serve Stroom through its Ingress, with HTTPS", func() {
		expectServedViaIngress(namespace, pod)
	})
}

// sampleValues returns the fixture values for Keycloak and the TLS Secrets, which the samples expect to exist
func sampleValues(namespace string) map[string]string {
	values := keycloakClusterValues(operatorDatabase, namespace)
	values["PROVISIONING_STROOM_TAG"] = stroomTag
	return values
}

// deploySamplePrerequisites creates the namespace, Keycloak, the TLS Secrets and everything in a sample except its
// StroomCluster, which it returns, so the cluster can be created once the database is ready and the operator's
// Stroom user exists
func deploySamplePrerequisites(namespace string, sample string, values map[string]string) string {
	createNamespace(namespace)
	for name, value := range generateTls(namespace) {
		values[name] = value
	}
	applyTestData(namespace, "tls-secrets.yaml", values)
	applyTestData(namespace, "keycloak.yaml", values)

	built := mustRun("bin/kustomize", "build", filepath.Join("test", "e2e", "testdata", "samples", sample))
	built = strings.NewReplacer(
		// The sample's placeholders, for the test's Keycloak and MySQL server
		"https://keycloak.example.com/realms/stroom", fmt.Sprintf("http://keycloak.%v.svc.cluster.local:8080/realms/stroom", namespace),
		"mysql.example.com", fmt.Sprintf("mysql.%v.svc.cluster.local", namespace),
		"replace-with-the-stroom-client-secret", stroomClientSecret,
		"replace-with-the-operator-client-secret", operatorClientSecret,
		"replace-with-the-stroomuser-password", values["EXTERNAL_MYSQL_PASSWORD"],
	).Replace(built)

	var cluster, others []string
	for _, document := range strings.Split(built, "\n---\n") {
		if strings.Contains(document, "\nkind: StroomCluster\n") {
			cluster = append(cluster, document)
		} else {
			others = append(others, document)
		}
	}
	Expect(cluster).To(HaveLen(1), "the sample should have one StroomCluster")
	applyManifest(namespace, strings.Join(others, "\n---\n"))
	waitForCondition(namespace, "deployment/keycloak", "condition=Available", defaultTimeout)
	return cluster[0]
}

// provisionSampleUsers creates the Stroom user for the operator, which also migrates the Stroom database, using the
// sample's Stroom configuration
func provisionSampleUsers(namespace string, values map[string]string) {
	By("creating the Stroom user for the operator, and migrating the database")
	applyTestData(namespace, "provision-users.yaml", values)
	waitForCondition(namespace, "job/provision-users", "condition=Complete", stroomStartTimeout)
}

// applyManifest applies manifests in a namespace
func applyManifest(namespace string, manifest string) {
	path := filepath.Join(GinkgoT().TempDir(), "manifest.yaml")
	ExpectWithOffset(1, os.WriteFile(path, []byte(manifest), 0o600)).To(Succeed())
	mustKubectl("apply", "-n", namespace, "-f", path)
}

// expectServedViaIngress expects Stroom's user interface to be served through ingress-nginx, which can take a few
// seconds after a node is ready to route to it
func expectServedViaIngress(namespace string, pod string) {
	EventuallyWithOffset(1, func() string {
		return sampleViaIngress(namespace, pod, "GET", "/")
	}, 2*time.Minute, 5*time.Second).Should(MatchRegexp(`^[23][0-9]{2}$`))
}

// sampleViaIngress requests a path from Stroom through ingress-nginx, as a browser would, returning the status code
func sampleViaIngress(namespace string, pod string, method string, path string) string {
	ingressIp := mustKubectl("get", "service", "-n", "ingress-nginx", "ingress-nginx-controller", "-o", "jsonpath={.spec.clusterIP}")
	return httpStatus(namespace, pod, "https://stroom.e2e.local"+path, "--resolve", "stroom.e2e.local:443:"+ingressIp, "-X", method)
}
