package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests deploy the operator to a kind cluster and use it to run Stroom. See README.md for how to run them.

const (
	operatorNamespace  = "stroom-operator-system"
	operatorDeployment = "deployment/stroom-operator-controller-manager"
	// operatorRepository is the image repository the Helm chart deploys the operator from
	operatorRepository = "ghcr.io/gradata-systems/stroom-k8s-operator"
	// previousRepository is the image repository of the previous release, from before the move to GHCR
	previousRepository = "gradata/stroom-k8s-operator"
	// previousManifestImage is the operator image the previous release's all-in-one manifest installs
	previousManifestImage = previousRepository + ":1.1.3"
)

var (
	kindCluster = envOrDefault("KIND_CLUSTER", "stroom-operator-e2e")
	// operatorTag is the tag of the operator image built and tested
	operatorTag = envOrDefault("E2E_OPERATOR_TAG", "e2e")
	// previousRelease is the operator release the upgrade suite upgrades from. Its charts come from the release's
	// Git tag, and its image from the image registry.
	previousRelease   = envOrDefault("E2E_PREVIOUS_RELEASE", "1.1.5")
	stroomTag         = envOrDefault("E2E_STROOM_TAG", "v7.13.2")
	previousStroomTag = envOrDefault("E2E_PREVIOUS_STROOM_TAG", "v7.13.1")
	keycloakImage     = envOrDefault("E2E_KEYCLOAK_IMAGE", "quay.io/keycloak/keycloak:26.0.7")
	stroomNodes       = envOrDefault("E2E_STROOM_NODES", "2")
)

// Images used by the tests, which are loaded into the kind cluster in advance so pods don't wait on image pulls
var testImages = []string{
	previousRepository + ":" + previousRelease,
	previousManifestImage,
	"gchq/stroom:" + stroomTag,
	"gchq/stroom:" + previousStroomTag,
	"mysql/mysql-server:8.0.32",
	"mysql:8.4",
	keycloakImage,
	pauseImage,
	"gchq/stroom-log-sender:" + logSenderTag,
}

// pauseImage is run in place of Stroom by tests that only need the operator to manage pods, not run Stroom
const pauseImage = "registry.k8s.io/pause:3.10"

func envOrDefault(name string, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}

func stroomNodeCount() int {
	count, err := strconv.Atoi(stroomNodes)
	if err != nil || count < 1 {
		panic(fmt.Sprintf("E2E_STROOM_NODES must be a positive integer, not %q", stroomNodes))
	}
	return count
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Running e2e tests on kind cluster %v\n", kindCluster)
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("checking the kind cluster exists")
	Expect(mustRun("kind", "get", "clusters")).To(ContainSubstring(kindCluster))

	operatorImage := operatorRepository + ":" + operatorTag
	if os.Getenv("E2E_SKIP_BUILD") != "1" {
		By("building the operator image")
		mustRun("make", "docker-build", "IMG="+operatorImage)
	}

	By("loading images into the kind cluster")
	for _, image := range append([]string{operatorImage}, testImages...) {
		if _, err := run("docker", "image", "inspect", image); err != nil {
			mustRun("docker", "pull", "-q", image)
		}
		mustRun("kind", "load", "docker-image", image, "--name", kindCluster)
	}
})

var _ = AfterSuite(func() {
	if CurrentSpecReport().Failed() || os.Getenv("E2E_KEEP_NAMESPACES") == "1" {
		return
	}
	By("removing the operator")
	removeOperator()
})

// deployOperator installs or upgrades the operator built from this repository, using its Helm charts. An operator
// installed some other way is removed first, as Helm won't take over its resources.
func deployOperator() {
	if _, err := run("helm", "--kube-context", "kind-"+kindCluster, "status", "stroom-operator", "-n", operatorNamespace); err != nil {
		removeOperator()
	}
	deployOperatorCharts(filepath.Join(projectDir, "charts"), operatorTag)
}

// deployPreviousRelease installs the previous operator release, using the Helm charts from its Git tag
func deployPreviousRelease() {
	chartsDir := GinkgoT().TempDir()
	mustRun("sh", "-c", fmt.Sprintf("git archive --format=tar %v charts | tar -x -C %v", previousRelease, chartsDir))
	deployOperatorCharts(filepath.Join(chartsDir, "charts"), previousRelease)
}

func deployOperatorCharts(chartsDir string, imageTag string) {
	absolute, err := filepath.Abs(chartsDir)
	Expect(err).ToNot(HaveOccurred())
	mustRun("helm", "--kube-context", "kind-"+kindCluster, "upgrade", "--install", "stroom-operator-crds",
		filepath.Join(absolute, "stroom-operator-crds"), "-n", operatorNamespace, "--create-namespace", "--wait")
	mustRun("helm", "--kube-context", "kind-"+kindCluster, "upgrade", "--install", "stroom-operator",
		filepath.Join(absolute, "stroom-operator"), "-n", operatorNamespace,
		"--set", "image.tag="+imageTag, "--set", "image.pullPolicy=IfNotPresent", "--wait", "--timeout", "5m")
	mustKubectl("rollout", "status", "-n", operatorNamespace, operatorDeployment, "--timeout=5m")
}

// deployPreviousReleaseManifest installs the operator from the all-in-one manifest at the previous release's Git tag,
// which releases up to 1.1.5 were published as. The 1.1.5 manifest was never regenerated, so it installs the 1.1.3
// CRDs and operator (previousManifestImage), as users of it have.
func deployPreviousReleaseManifest() {
	manifest := mustRun("git", "show", previousRelease+":deploy/all-in-one.yaml")
	path := filepath.Join(GinkgoT().TempDir(), "all-in-one.yaml")
	Expect(os.WriteFile(path, []byte(manifest), 0o600)).To(Succeed())
	mustKubectl("apply", "--server-side", "-f", path)
	waitForCondition(operatorNamespace, operatorDeployment, "condition=Available", defaultTimeout)
}

// operatorKinds are the plural names of the operator's resource kinds, as its CRDs are named
var operatorKinds = []string{"stroomclusters", "databaseservers", "databasebackups", "stroomtaskautoscalers"}

// removeOperator uninstalls the operator, however it was installed. Removing the CRDs removes any remaining
// resources of those kinds, such as those left by an interrupted run. The controller is stopped first, then the
// resources' finalizers are removed, as with the controller stopped nothing else will, and while it runs it would
// add them back.
func removeOperator() {
	_, _ = run("helm", "--kube-context", "kind-"+kindCluster, "uninstall", "stroom-operator", "-n", operatorNamespace, "--wait")
	_, _ = kubectl("delete", operatorDeployment, "-n", operatorNamespace, "--ignore-not-found", "--wait=true", "--timeout=5m")
	removeFinalizers()

	_, _ = run("helm", "--kube-context", "kind-"+kindCluster, "uninstall", "stroom-operator-crds", "-n", operatorNamespace, "--wait")
	if manifests, err := run("bin/kustomize", "build", "config/default"); err == nil {
		path := filepath.Join(GinkgoT().TempDir(), "operator.yaml")
		Expect(os.WriteFile(path, []byte(manifests), 0o600)).To(Succeed())
		_, _ = kubectl("delete", "--ignore-not-found", "--wait=true", "--timeout=5m", "-f", path)
	}
	_, _ = kubectl("delete", "namespace", operatorNamespace, "--ignore-not-found", "--wait=true", "--timeout=5m")
	for _, kind := range operatorKinds {
		_, _ = kubectl("delete", "crd", kind+".stroom.gchq.github.io", "--ignore-not-found", "--wait=false")
	}
	// Resources being deleted with the CRDs may still have finalizers
	removeFinalizers()
	for _, kind := range operatorKinds {
		_, _ = kubectl("wait", "--for=delete", "crd/"+kind+".stroom.gchq.github.io", "--timeout=5m")
	}
}

// removeFinalizers removes the finalizers from every resource of the operator's kinds
func removeFinalizers() {
	for _, kind := range operatorKinds {
		resources, err := kubectl("get", kind+".stroom.gchq.github.io", "-A", "-o",
			`jsonpath={range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}`)
		if err != nil {
			continue
		}
		for _, resource := range strings.Fields(resources) {
			namespace, name, _ := strings.Cut(resource, "/")
			_, _ = kubectl("patch", kind+".stroom.gchq.github.io", name, "-n", namespace, "--type=merge",
				"-p", `{"metadata":{"finalizers":null}}`)
		}
	}
}

// operatorLogs returns the operator's logs
func operatorLogs() string {
	output, _ := kubectl("logs", "-n", operatorNamespace, operatorDeployment)
	return output
}

// restartOperator deletes the operator pod, and waits for its replacement to be running
func restartOperator() {
	mustKubectl("delete", "pod", "-n", operatorNamespace, "-l", "control-plane=controller-manager", "--wait=true")
	mustKubectl("rollout", "status", "-n", operatorNamespace, operatorDeployment, "--timeout=5m")
	waitForCondition(operatorNamespace, operatorDeployment, "condition=Available", defaultTimeout)
}
