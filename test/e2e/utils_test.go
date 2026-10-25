package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// projectDir is the repository root, relative to this package
const projectDir = "../.."

// run executes a command from the repository root, returning its combined output
func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = projectDir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %v %v\n", name, strings.Join(args, " "))
	err := cmd.Run()
	if err != nil {
		return output.String(), fmt.Errorf("%v %v failed: %w\n%v", name, strings.Join(args, " "), err, output.String())
	}
	return output.String(), nil
}

// mustRun executes a command, failing the spec if it fails
func mustRun(name string, args ...string) string {
	output, err := run(name, args...)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return output
}

// kubectl runs kubectl against the e2e cluster
func kubectl(args ...string) (string, error) {
	return run("kubectl", append([]string{"--context", "kind-" + kindCluster}, args...)...)
}

// mustKubectl runs kubectl against the e2e cluster, failing the spec if it fails
func mustKubectl(args ...string) string {
	output, err := kubectl(args...)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return output
}

// applyTestData applies a manifest from testdata to a namespace, after substituting ${VAR} references with values.
// References to variables that aren't provided are an error, so a fixture can't silently be applied incomplete. $$
// is a literal $, for example for a shell variable in a command.
func applyTestData(namespace string, file string, values map[string]string) {
	content, err := os.ReadFile(filepath.Join("testdata", file))
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	var missing []string
	manifest := os.Expand(string(content), func(name string) string {
		if name == "$" {
			return "$"
		}
		if value, ok := values[name]; ok {
			return value
		}
		missing = append(missing, name)
		return ""
	})
	ExpectWithOffset(1, missing).To(BeEmpty(), "unresolved variables in %v", file)

	path := filepath.Join(GinkgoT().TempDir(), file)
	ExpectWithOffset(1, os.WriteFile(path, []byte(manifest), 0o600)).To(Succeed())
	absolute, err := filepath.Abs(path)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	_, err = kubectl("apply", "-n", namespace, "-f", absolute)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
}

// createNamespace creates a namespace, deleting it once the enclosing container's specs complete. A namespace left
// by an interrupted run is deleted first.
func createNamespace(namespace string) {
	_, _ = kubectl("delete", "namespace", namespace, "--ignore-not-found", "--wait=true", "--timeout=10m")
	mustKubectl("create", "namespace", namespace)
	DeferCleanup(func() {
		if CurrentSpecReport().Failed() {
			saveDiagnostics(namespace)
		}
		if os.Getenv("E2E_KEEP_NAMESPACES") == "1" {
			return
		}
		_, _ = kubectl("delete", "namespace", namespace, "--wait=true", "--timeout=10m")
	})
}

// saveDiagnostics writes a namespace's resources, events, pod descriptions and logs to a directory under
// E2E_DIAGNOSTICS_DIR, if it's set, before the namespace is deleted
func saveDiagnostics(namespace string) {
	root := os.Getenv("E2E_DIAGNOSTICS_DIR")
	if root == "" {
		return
	}
	dir := filepath.Join(root, namespace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "saving diagnostics: %v\n", err)
		return
	}
	save := func(file string, args ...string) {
		output, _ := kubectl(append([]string{"-n", namespace}, args...)...)
		_ = os.WriteFile(filepath.Join(dir, file), []byte(output), 0o644)
	}
	save("resources.txt", "get", "all,pvc,configmaps,secrets", "-o", "wide")
	save("events.txt", "get", "events", "--sort-by=.lastTimestamp")
	save("stroom-resources.yaml", "get", strings.Join(operatorKinds, ","), "-o", "yaml")
	save("pods.txt", "describe", "pods")
	pods, _ := kubectl("get", "pods", "-n", namespace, "-o", "jsonpath={.items[*].metadata.name}")
	for _, pod := range strings.Fields(pods) {
		save("logs-"+pod+".txt", "logs", pod, "--all-containers", "--prefix")
		save("logs-previous-"+pod+".txt", "logs", pod, "--all-containers", "--prefix", "--previous")
	}
	// The operator, which is removed when the suite ends
	operatorLog, _ := kubectl("logs", "-n", operatorNamespace, "deployment/stroom-operator-controller-manager",
		"--all-containers", "--tail=2000")
	_ = os.WriteFile(filepath.Join(dir, "logs-operator.txt"), []byte(operatorLog), 0o644)
	_, _ = fmt.Fprintf(GinkgoWriter, "saved diagnostics for namespace %v to %v\n", namespace, dir)
}

// podRestartCount returns the restart count of a pod's stroom-node container
func podRestartCount(namespace string, pod string) int {
	output, err := kubectl("get", "pod", "-n", namespace, pod, "-o",
		`jsonpath={.status.containerStatuses[?(@.name=="stroom-node")].restartCount}`)
	if err != nil || strings.TrimSpace(output) == "" {
		// The pod, or its container's status, doesn't exist yet
		return 0
	}
	var count int
	_, err = fmt.Sscanf(strings.TrimSpace(output), "%d", &count)
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), "restart count of %v: %q", pod, output)
	return count
}

// expectNodeRestarted waits for a Stroom node's container to be restarted, with a message in its log. The log is
// that of the last or current container, as the container may be restarting when it's read.
func expectNodeRestarted(namespace string, pod string, message string) {
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(podRestartCount(namespace, pod)).To(BeNumerically(">=", 1))
		previous, _ := kubectl("logs", "-n", namespace, pod, "-c", "stroom-node", "--previous")
		current, _ := kubectl("logs", "-n", namespace, pod, "-c", "stroom-node")
		g.Expect(previous + current).To(ContainSubstring(message))
	}, defaultTimeout, 10*time.Second).Should(Succeed())
}

// replaceNodePod waits for the operator to update a Stroom node's StatefulSet, until a field of it (a JSONPath)
// contains a value, then deletes the node's pod so it's recreated from the update. The StatefulSet's rolling update
// would replace it anyway, but not until a crashing pod's back-off has passed.
func replaceNodePod(namespace string, pod string, field string, value string) {
	statefulSet := pod[:strings.LastIndex(pod, "-")]
	EventuallyWithOffset(1, func() string {
		return mustKubectl("get", "statefulset", "-n", namespace, statefulSet, "-o", "jsonpath="+field)
	}, 2*time.Minute, 5*time.Second).Should(ContainSubstring(value))

	uid := mustKubectl("get", "pod", "-n", namespace, pod, "-o", "jsonpath={.metadata.uid}")
	mustKubectl("delete", "pod", "-n", namespace, pod, "--wait=false")
	EventuallyWithOffset(1, func() string {
		output, _ := kubectl("get", "pod", "-n", namespace, pod, "-o", "jsonpath={.metadata.uid}")
		return output
	}, 2*time.Minute, 2*time.Second).ShouldNot(Or(BeEmpty(), Equal(uid)))
}

// execInPod runs a shell command in a pod's container
func execInPod(namespace string, pod string, container string, command string) (string, error) {
	return kubectl("exec", "-n", namespace, pod, "-c", container, "--", "bash", "-c", command)
}

// waitForCondition waits for a resource to reach a condition, using kubectl wait
func waitForCondition(namespace string, resource string, condition string, timeout time.Duration) {
	EventuallyWithOffset(1, func() error {
		// The resource may not exist yet, so retry until it does
		_, err := kubectl("wait", "-n", namespace, resource, "--for", condition, "--timeout", "30s")
		return err
	}, timeout, 10*time.Second).Should(Succeed(), "waiting for %v to be %v", resource, condition)
}

// stroomPodNames returns the names of a StroomCluster node set's pods
func stroomPodNames(cluster string, nodeSet string, count int) []string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("stroom-%v-node-%v-%v", cluster, nodeSet, i)
	}
	return names
}
