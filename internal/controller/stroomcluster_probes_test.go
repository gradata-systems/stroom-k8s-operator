package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

var _ = Describe("StroomCluster probes", func() {

	reconciler := &StroomClusterReconciler{Scheme: scheme.Scheme}

	Context("createProbe()", func() {
		timings := stroomv1.ProbeTimings{
			InitialDelaySeconds: 1,
			TimeoutSeconds:      2,
			PeriodSeconds:       3,
			SuccessThreshold:    4,
			FailureThreshold:    5,
		}

		It("should query the unfiltered health check endpoint on the admin port", func() {
			// BASELINE: known issue, see #62. All health checks run, including the identity provider check, so an
			// identity provider outage fails the liveness probe of every node.
			probe := reconciler.createProbe(&timings, AdminPortName)
			Expect(probe.HTTPGet.Path).Should(Equal("/stroomAdmin/filteredhealthcheck"))
			Expect(probe.HTTPGet.Port.StrVal).Should(Equal(AdminPortName))
		})
		It("should apply the probe timings", func() {
			probe := reconciler.createProbe(&timings, AdminPortName)
			Expect(probe.InitialDelaySeconds).Should(Equal(int32(1)))
			Expect(probe.TimeoutSeconds).Should(Equal(int32(2)))
			Expect(probe.SuccessThreshold).Should(Equal(int32(4)))
			Expect(probe.FailureThreshold).Should(Equal(int32(5)))
		})
		It("should set the period from the initial delay", func() {
			// BASELINE: known bug, see #62. periodSeconds is ignored.
			probe := reconciler.createProbe(&timings, AdminPortName)
			Expect(probe.PeriodSeconds).Should(Equal(int32(1)))
		})
		It("should leave unset timings to the Kubernetes defaults", func() {
			probe := reconciler.createProbe(&stroomv1.ProbeTimings{}, AdminPortName)
			Expect(probe.InitialDelaySeconds).Should(BeZero())
			Expect(probe.PeriodSeconds).Should(BeZero())
			Expect(probe.FailureThreshold).Should(BeZero())
		})
	})

	Context("startup probe", func() {
		newCluster := func(policy appsv1.PodManagementPolicyType, count int32) (*stroomv1.StroomCluster, *stroomv1.NodeSet) {
			cluster := &stroomv1.StroomCluster{Spec: stroomv1.StroomClusterSpec{
				PodManagementPolicy: policy,
				NodeSets: []stroomv1.NodeSet{{
					Name:  "data",
					Count: count,
					StartupProbeTimings: stroomv1.ProbeTimings{
						InitialDelaySeconds: 5,
						TimeoutSeconds:      5,
						PeriodSeconds:       5,
						SuccessThreshold:    1,
						FailureThreshold:    10,
					},
					MemoryOptions: stroomv1.JvmMemoryOptions{InitialPercentage: 50, MaxPercentage: 75},
				}},
			}}
			return cluster, &cluster.Spec.NodeSets[0]
		}

		It("should run the node startup script", func() {
			cluster, nodeSet := newCluster(appsv1.OrderedReadyPodManagement, 1)
			probe := reconciler.createStatefulSet(cluster, nodeSet, goldenDatabaseConnectionInfo("db")).Spec.Template.Spec.Containers[0].StartupProbe
			Expect(probe.Exec.Command).Should(Equal([]string{"bash", "/stroom/scripts/node-start.sh"}))
			Expect(probe.PeriodSeconds).Should(Equal(int32(5)))
		})
		It("should use the failure threshold as-is, regardless of how many nodes start in parallel", func() {
			// BASELINE: nodes waiting on the Stroom bootstrap lock get the same startup time as the first node
			cluster, nodeSet := newCluster(appsv1.ParallelPodManagement, 3)
			probe := reconciler.createStatefulSet(cluster, nodeSet, goldenDatabaseConnectionInfo("db")).Spec.Template.Spec.Containers[0].StartupProbe
			Expect(probe.FailureThreshold).Should(Equal(int32(10)))
		})
	})
})
