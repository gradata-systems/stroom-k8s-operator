package e2e

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const statefulSet = "statefulset/stroom-e2e-node-node"

// operatorUpgradeSpecs tests upgrading the operator from the previous release, with a Stroom cluster running
func operatorUpgradeSpecs() {
	const namespace = "stroom-e2e-upgrade"
	pods := stroomPodNames("e2e", "node", stroomNodeCount())
	values := keycloakClusterValues(operatorDatabase, namespace)

	BeforeAll(func() {
		By("installing operator " + previousRelease)
		removeOperator()
		deployPreviousRelease()

		deployKeycloakAndDatabase(operatorDatabase, namespace, values)
		applyTestData(namespace, "stroom-cluster.yaml", values)
	})

	It("should run Stroom with the previous release", func() {
		Expect(operatorImage()).To(Equal(operatorRepository + ":" + previousRelease))
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
		}
	})

	It("should keep the nodes running when the operator is upgraded", func() {
		uids := podUids(namespace, pods)
		revision := mustKubectl("get", statefulSet, "-n", namespace, "-o", "jsonpath={.status.updateRevision}")

		By("upgrading the operator")
		deployOperator()
		Expect(operatorImage()).To(Equal(operatorRepository + ":" + operatorTag))
		Eventually(operatorLogs, 2*time.Minute, 5*time.Second).Should(ContainSubstring("StatefulSet reconciled"))

		// BASELINE: the previous release renders the same pod template as this build, so the nodes are untouched.
		// A change to the pod template rolls each node set.
		Consistently(func() map[string]string {
			return podUids(namespace, pods)
		}, time.Minute, 10*time.Second).Should(Equal(uids))
		Expect(mustKubectl("get", statefulSet, "-n", namespace, "-o", "jsonpath={.status.updateRevision}")).To(Equal(revision))
	})

	It("should keep the StroomCluster's stored probe timings", func() {
		timings := mustKubectl("get", "stroomcluster", "e2e", "-n", namespace, "-o",
			"jsonpath={.spec.nodeSets[0].startupProbeTimings.failureThreshold} {.spec.nodeSets[0].livenessProbeTimings.failureThreshold}")
		Expect(timings).To(Equal(values["STARTUP_FAILURE_THRESHOLD"] + " 10"))
	})
}

// bootstrapLockSpecs tests nodes waiting on the Stroom bootstrap lock, which a node holds while it upgrades the
// database, then upgrades Stroom. The lock is held by the test for a fixed time, so the wait is predictable.
func bootstrapLockSpecs() {
	const (
		namespace = "stroom-e2e-lock"
		// The startup probe allows 10 + 9 x 10 seconds, which the lock is held for longer than
		startupInitialDelay     = "10"
		startupFailureThreshold = "9"
		lockHoldSeconds         = 150
	)
	pods := stroomPodNames("e2e", "node", stroomNodeCount())
	values := keycloakClusterValues(operatorDatabase, namespace)
	values["PROVISIONING_STROOM_TAG"] = previousStroomTag
	values["STROOM_TAG"] = previousStroomTag
	values["STARTUP_INITIAL_DELAY"] = startupInitialDelay
	values["STARTUP_FAILURE_THRESHOLD"] = startupFailureThreshold

	BeforeAll(func() {
		deployKeycloakAndDatabase(operatorDatabase, namespace, values)
	})

	It("should restart nodes that wait on the bootstrap lock for longer than the startup probe allows", func() {
		By("making the nodes upgrade the database, while the lock is held")
		holdBootstrapLock(namespace, lockHoldSeconds)
		applyTestData(namespace, "stroom-cluster.yaml", values)

		// BASELINE: the startup probe doesn't allow for the lock wait, so the nodes are restarted while waiting
		Eventually(func(g Gomega) {
			for _, pod := range pods {
				g.Expect(podRestartCount(namespace, pod)).To(BeNumerically(">=", 1), pod)
			}
		}, lockHoldSeconds*time.Second+3*time.Minute, 10*time.Second).Should(Succeed())

		By("waiting for the nodes to start once the lock is released")
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
			// Logs are kept on the node's volume, so include those of restarted containers, some of which are archived
			// zcat exits with an error for a file being written as it's read, so only its output is checked
			output, err := execInPod(namespace, pod, "stroom-node", "zcat -f /stroom/logs/app/app* 2> /dev/null; true")
			Expect(err).ToNot(HaveOccurred())
			Expect(output).To(ContainSubstring("Waiting to acquire bootstrap lock"))
		}
	})

	It("should upgrade Stroom one node at a time, draining each node", func() {
		drains := map[string]int{}
		for _, pod := range pods {
			drains[pod] = drainCount(namespace, pod)
		}

		mustKubectl("patch", "stroomcluster", "e2e", "-n", namespace, "--type=json",
			"-p", fmt.Sprintf(`[{"op": "replace", "path": "/spec/image/tag", "value": %q}]`, stroomTag))
		mustKubectl("rollout", "status", statefulSet, "-n", namespace, "--timeout=30m")

		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
			image := mustKubectl("get", "pod", pod, "-n", namespace, "-o",
				`jsonpath={.spec.containers[?(@.name=="stroom-node")].image}`)
			Expect(image).To(Equal("gchq/stroom:" + stroomTag))
			Expect(drainCount(namespace, pod)).To(BeNumerically(">", drains[pod]), "%v should have drained before upgrading", pod)
		}
		Expect(mysqlAsRoot(namespace, "e2e", rootPassword(namespace, "e2e"), "SELECT build_version FROM stroom.build_version")).
			To(ContainSubstring(strings.TrimPrefix(stroomTag, "v")))
	})
}

// holdBootstrapLock makes Stroom nodes upgrade the database when they start, by changing the build version recorded
// in it, then holds the lock they need to do so for a number of seconds
func holdBootstrapLock(namespace string, seconds int) {
	password := rootPassword(namespace, "e2e")
	mysqlAsRoot(namespace, "e2e", password, "UPDATE stroom.build_version SET build_version = 'e2e-forced-upgrade'")

	// Run the session in the background, so it outlives this command
	mustKubectl("exec", "-n", namespace, "stroom-e2e-db-0", "-c", "mysql", "--", "sh", "-c", fmt.Sprintf(
		`MYSQL_PWD='%v' nohup mysql -uroot -e "BEGIN; SELECT * FROM stroom.build_version FOR UPDATE; DO SLEEP(%v); COMMIT;" > /tmp/bootstrap-lock.log 2>&1 &`,
		password, seconds))
	Eventually(func() string {
		return strings.TrimSpace(mysqlAsRoot(namespace, "e2e", password,
			"SELECT COUNT(*) FROM performance_schema.data_locks WHERE OBJECT_SCHEMA = 'stroom' AND OBJECT_NAME = 'build_version'"))
	}, time.Minute, 2*time.Second).ShouldNot(Equal("0"), "the bootstrap lock should be held")
}

// drainCount returns how many times a node's pre-stop hook has drained it
func drainCount(namespace string, pod string) int {
	output, _ := execInPod(namespace, pod, "stroom-node", "cat /stroom/logs/k8s/pre-stop.log 2>/dev/null")
	return strings.Count(output, "All tasks drained")
}

// operatorImage returns the image the operator is running
func operatorImage() string {
	return mustKubectl("get", operatorDeployment, "-n", operatorNamespace, "-o",
		`jsonpath={.spec.template.spec.containers[?(@.name=="manager")].image}`)
}

// podUids returns the UID of each pod, which changes when a pod is replaced
func podUids(namespace string, pods []string) map[string]string {
	uids := map[string]string{}
	for _, pod := range pods {
		uids[pod] = mustKubectl("get", "pod", "-n", namespace, pod, "-o", "jsonpath={.metadata.uid}")
	}
	return uids
}
