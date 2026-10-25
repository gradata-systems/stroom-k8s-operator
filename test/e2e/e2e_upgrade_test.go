package e2e

import (
	"fmt"
	"path/filepath"
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
	values["LOG_SENDER"] = disabledLogSender

	BeforeAll(func() {
		By("installing operator " + previousRelease)
		removeOperator()
		deployPreviousRelease()

		deployKeycloakAndDatabase(operatorDatabase, namespace, values)
		applyTestData(namespace, "stroom-cluster.yaml", values)
	})

	It("should run Stroom with the previous release", func() {
		Expect(operatorImage()).To(Equal(previousRepository + ":" + previousRelease))
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
		}
	})

	It("should update the database server, then roll the nodes, draining each, when the operator is upgraded", func() {
		const dbPod = "stroom-e2e-db-0"
		uids := podUids(namespace, append([]string{dbPod}, pods...))
		drains := map[string]int{}
		for _, pod := range pods {
			drains[pod] = drainCount(namespace, pod)
		}
		revision := mustKubectl("get", statefulSet, "-n", namespace, "-o", "jsonpath={.status.updateRevision}")

		By("upgrading the operator")
		deployOperator()
		Expect(operatorImage()).To(Equal(operatorRepository + ":" + operatorTag))

		// This release changes the database server's pod template, and the nodes' (the probes and security settings).
		// The database server is updated first, as the nodes need it to drain.
		Eventually(func() string {
			output, _ := kubectl("get", "pod", "-n", namespace, dbPod, "-o", "jsonpath={.metadata.uid}")
			return output
		}, defaultTimeout, 5*time.Second).ShouldNot(Or(BeEmpty(), Equal(uids[dbPod])), "the database server should be replaced")
		waitForCondition(namespace, "pod/"+dbPod, "condition=Ready", defaultTimeout)
		dbStarted := mustKubectl("get", "pod", "-n", namespace, dbPod, "-o", "jsonpath={.metadata.creationTimestamp}")

		Eventually(func() string {
			return mustKubectl("get", statefulSet, "-n", namespace, "-o", "jsonpath={.status.updateRevision}")
		}, 2*time.Minute, 5*time.Second).ShouldNot(Equal(revision))
		mustKubectl("rollout", "status", statefulSet, "-n", namespace, "--timeout=30m")
		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
			Expect(podUids(namespace, []string{pod})[pod]).ToNot(Equal(uids[pod]), "%v should have been replaced", pod)
			// RFC 3339 timestamps in UTC sort as strings
			nodeStarted := mustKubectl("get", "pod", "-n", namespace, pod, "-o", "jsonpath={.metadata.creationTimestamp}")
			Expect(nodeStarted >= dbStarted).To(BeTrue(), "%v (%v) should have been replaced after the database server (%v)", pod, nodeStarted, dbStarted)
			Expect(drainCount(namespace, pod)).To(BeNumerically(">", drains[pod]), "%v should have drained:\n%v", pod,
				lifecycleLogs(namespace, pod))
		}
	})

	It("should give the nodes the new health check filters", func() {
		// The StroomCluster has no allow or deny lists stored, as the previous release had none, so the defaults apply
		for _, pod := range pods {
			paths := mustKubectl("get", "pod", "-n", namespace, pod, "-o",
				`jsonpath={.spec.containers[?(@.name=="stroom-node")].livenessProbe.httpGet.path} {.spec.containers[?(@.name=="stroom-node")].readinessProbe.httpGet.path}`)
			Expect(paths).To(Equal("/stroomAdmin/filteredhealthcheck?allow=deadlocks /stroomAdmin/filteredhealthcheck?deny=" + externalIdpCheck))
		}
	})

	It("should keep the StroomCluster's stored probe timings", func() {
		timings := mustKubectl("get", "stroomcluster", "e2e", "-n", namespace, "-o",
			"jsonpath={.spec.nodeSets[0].startupProbeTimings.failureThreshold} {.spec.nodeSets[0].livenessProbeTimings.failureThreshold}")
		Expect(timings).To(Equal(values["STARTUP_FAILURE_THRESHOLD"] + " 10"))
	})
}

// manifestMigrationSpecs tests moving an operator installed from the previous release's all-in-one manifest to the
// Helm charts, as docs/operations/upgrading.md describes, with a StroomCluster running. The cluster runs the pause
// image, as only its resources need to survive.
func manifestMigrationSpecs() {
	const namespace = "stroom-e2e-manifest"
	const pod = "stroom-e2e-node-node-0"
	uid := func(args ...string) string {
		return mustKubectl(append([]string{"get", "-o", "jsonpath={.metadata.uid}"}, args...)...)
	}
	var uids map[string]string

	BeforeAll(func() {
		By("installing operator " + previousRelease + " from its all-in-one manifest")
		removeOperator()
		deployPreviousReleaseManifest()

		createNamespace(namespace)
		applyTestData(namespace, "secrets.yaml", keycloakClusterValues(operatorDatabase, namespace))
		applyTestData(namespace, "database-server.yaml", map[string]string{
			"DATABASE_SERVER_NAME": "e2e", "MYSQL_REPOSITORY": "mysql/mysql-server", "MYSQL_TAG": "8.0.32",
		})
		// The manifest's operator only lets a StroomCluster use a DatabaseServer with the same name, and never deploys
		// one without a log sender. Its CRD requires https and the log sender's mtlsEnabled.
		applyTestData(namespace, "stroom-cluster-pause.yaml", map[string]string{
			"CLUSTER_NAME": "e2e", "DATABASE_SERVER_REF": "{serverRef: {name: e2e}}", "NODE_COUNT": "1",
			"VOLUME_CLAIM_DELETE_POLICY": "# No volumeClaimDeletePolicy", "HTTPS": "https: {enabled: false}",
			"LOG_SENDER": "logSender: {enabled: false, mtlsEnabled: false, image: {repository: gchq/stroom-log-sender, tag: " + logSenderTag + "}}",
		})
	})

	It("should run the operator installed from the manifest", func() {
		Expect(operatorImage()).To(Equal(previousManifestImage))
		waitForCondition(namespace, "pod/stroom-e2e-db-0", "condition=Ready", defaultTimeout)
		// The manifest's operator drops false values when it updates a StroomCluster, so its CRD rejects the update of
		// any cluster without HTTPS or log sender mTLS, and it never deploys the nodes. Moving to Helm fixes that.
		Eventually(func() string {
			output, _ := kubectl("logs", "-n", operatorNamespace, "deployment/stroom-operator-controller-manager")
			return output
		}, 2*time.Minute, 5*time.Second).Should(ContainSubstring("spec.https.enabled: Required value"))
		uids = map[string]string{"stroomcluster": uid("stroomcluster", "e2e", "-n", namespace)}
		for _, crd := range operatorKinds {
			uids[crd] = uid("crd", crd+".stroom.gchq.github.io")
		}
	})

	It("should move the operator to the Helm charts, keeping the CRDs and the StroomCluster", func() {
		mustRun("bash", filepath.Join("test", "e2e", "testdata", "adopt-manifest-install.sh"), "kind-"+kindCluster)
		deployOperatorCharts(filepath.Join(projectDir, "charts"), operatorTag)
		Expect(operatorImage()).To(Equal(operatorRepository + ":" + operatorTag))
		for _, release := range []string{"stroom-operator-crds", "stroom-operator"} {
			Expect(mustRun("helm", "--kube-context", "kind-"+kindCluster, "status", release, "-n", operatorNamespace)).
				To(ContainSubstring("STATUS: deployed"), release)
		}

		for _, crd := range operatorKinds {
			Expect(uid("crd", crd+".stroom.gchq.github.io")).To(Equal(uids[crd]), "the %v CRD should be kept", crd)
		}
		Expect(uid("stroomcluster", "e2e", "-n", namespace)).To(Equal(uids["stroomcluster"]))

		// The new operator reconciles the cluster, and deploys its nodes with this release's pod template
		Eventually(func() string {
			// The StatefulSet doesn't exist until the new operator creates it
			output, _ := kubectl("get", "statefulset", "stroom-e2e-node-node", "-n", namespace, "-o",
				`jsonpath={.spec.template.spec.containers[?(@.name=="stroom-node")].livenessProbe.httpGet.path}`)
			return output
		}, 5*time.Minute, 5*time.Second).Should(HaveSuffix("?allow=deadlocks"))
		waitForCondition(namespace, "pod/"+pod, "jsonpath={.status.phase}=Running", defaultTimeout)
	})
}

// bootstrapLockSpecs tests nodes waiting on the Stroom bootstrap lock, which a node holds while it upgrades the
// database, then upgrades Stroom. The lock is held by the test for a fixed time, so the wait is predictable.
func bootstrapLockSpecs() {
	const (
		namespace = "stroom-e2e-lock"
		// Without the lock wait allowance, the startup probe allows 10 + 9 x 10 seconds, which the lock is held for
		// longer than. The allowance adds 300 seconds for the second node, so 30 more failures.
		startupInitialDelay      = "10"
		startupFailureThreshold  = "9"
		lockWaitAllowance        = "300"
		expectedFailureThreshold = "39"
		lockHoldSeconds          = 150
	)
	// The allowance is for each node after the first, so at least two are needed
	nodes := max(2, stroomNodeCount())
	pods := stroomPodNames("e2e", "node", nodes)
	values := keycloakClusterValues(operatorDatabase, namespace)
	values["PROVISIONING_STROOM_TAG"] = previousStroomTag
	values["STROOM_TAG"] = previousStroomTag
	values["STROOM_NODES"] = fmt.Sprint(nodes)
	values["STARTUP_INITIAL_DELAY"] = startupInitialDelay
	values["STARTUP_FAILURE_THRESHOLD"] = startupFailureThreshold
	values["LOCK_WAIT_ALLOWANCE"] = "lockWaitAllowanceSeconds: " + lockWaitAllowance

	BeforeAll(func() {
		deployKeycloakAndDatabase(operatorDatabase, namespace, values)
	})

	It("should let nodes wait on the bootstrap lock without restarting them", func() {
		By("making the nodes upgrade the database, while the lock is held")
		holdBootstrapLock(namespace, lockHoldSeconds)
		applyTestData(namespace, "stroom-cluster.yaml", values)

		for _, pod := range pods {
			waitForCondition(namespace, "pod/"+pod, "condition=Ready", stroomStartTimeout)
			threshold := mustKubectl("get", "pod", "-n", namespace, pod, "-o",
				`jsonpath={.spec.containers[?(@.name=="stroom-node")].startupProbe.failureThreshold}`)
			Expect(threshold).To(Equal(expectedFailureThreshold), pod)
			Expect(podRestartCount(namespace, pod)).To(Equal(0), "%v should not have been restarted", pod)

			// zcat exits with an error for a file being written as it's read, so only its output is checked
			output, err := execInPod(namespace, pod, "stroom-node", "zcat -f /stroom/logs/app/app* 2> /dev/null; true")
			Expect(err).ToNot(HaveOccurred())
			Expect(output).To(ContainSubstring("Waiting to acquire bootstrap lock"), "%v should have waited on the lock", pod)
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

// lifecycleLogs returns a node's lifecycle script logs, which are kept on its volume, for diagnosing a failed spec
func lifecycleLogs(namespace string, pod string) string {
	output, err := execInPod(namespace, pod, "stroom-node", "ls -la /stroom/logs/k8s; tail -n 30 /stroom/logs/k8s/*.log")
	if err != nil {
		return fmt.Sprintf("%v: %v", err, output)
	}
	return output
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
