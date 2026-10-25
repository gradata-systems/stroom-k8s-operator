package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("StroomCluster probe timings", func() {

	newCluster := func(nodeSet map[string]interface{}) *unstructured.Unstructured {
		nodeSet["name"] = "nodeset-1"
		nodeSet["count"] = int64(1)
		nodeSet["localDataVolumeClaim"] = map[string]interface{}{}
		nodeSet["resources"] = map[string]interface{}{}
		nodeSet["memoryOptions"] = map[string]interface{}{"initialPercentage": int64(50), "maxPercentage": int64(75)}

		cluster := &unstructured.Unstructured{}
		cluster.SetAPIVersion(GroupVersion.String())
		cluster.SetKind("StroomCluster")
		cluster.SetName("stroom-" + rand.String(5))
		cluster.SetNamespace("default")
		cluster.Object["spec"] = map[string]interface{}{
			"image":             map[string]interface{}{"repository": "gchq/stroom"},
			"databaseServerRef": map[string]interface{}{},
			"openId":            map[string]interface{}{"clientId": "", "clientSecret": map[string]interface{}{"secretName": "", "key": ""}},
			"ingress":           map[string]interface{}{"hostName": "stroom.example.com"},
			// A typed update of a StroomCluster without a log sender fails validation, which the update test does
			"logSender": map[string]interface{}{"image": map[string]interface{}{"repository": "gchq/stroom-log-sender"}},
			"nodeSets":  []interface{}{nodeSet},
		}
		return cluster
	}

	createAndFetch := func(cluster *unstructured.Unstructured) *StroomCluster {
		Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())
		})

		fetched := &StroomCluster{}
		key := types.NamespacedName{Name: cluster.GetName(), Namespace: cluster.GetNamespace()}
		Expect(k8sClient.Get(context.Background(), key, fetched)).To(Succeed())
		return fetched
	}

	It("should default the database names", func() {
		cluster := createAndFetch(newCluster(map[string]interface{}{}))
		Expect(cluster.Spec.AppDatabaseName).Should(Equal("stroom"))
		Expect(cluster.Spec.StatsDatabaseName).Should(Equal("stats"))
	})

	It("should apply probe defaults when probe timings are omitted", func() {
		nodeSet := createAndFetch(newCluster(map[string]interface{}{})).Spec.NodeSets[0]

		Expect(nodeSet.LivenessProbeTimings.Allow).Should(HaveValue(Equal([]string{"deadlocks"})))
		Expect(nodeSet.LivenessProbeTimings.Deny).Should(SatisfyAny(BeNil(), HaveValue(BeEmpty())))
		Expect(nodeSet.LivenessProbeTimings.PeriodSeconds).Should(Equal(int32(5)))
		Expect(nodeSet.LivenessProbeTimings.FailureThreshold).Should(Equal(int32(10)))

		Expect(nodeSet.ReadinessProbeTimings.Allow).Should(SatisfyAny(BeNil(), HaveValue(BeEmpty())))
		Expect(nodeSet.ReadinessProbeTimings.Deny).Should(HaveValue(Equal([]string{"stroom.security.common.impl.ExternalIdpConfigurationProvider"})))
		Expect(nodeSet.ReadinessProbeTimings.PeriodSeconds).Should(Equal(int32(5)))

		Expect(nodeSet.StartupProbeTimings.FailureThreshold).Should(Equal(int32(24)))
		Expect(nodeSet.StartupProbeTimings.PeriodSeconds).Should(Equal(int32(5)))
		Expect(nodeSet.StartupProbeTimings.LockWaitAllowanceSeconds).Should(Equal(int32(30)))
	})

	It("should apply probe defaults to empty probe timings, as used by existing resources", func() {
		nodeSet := createAndFetch(newCluster(map[string]interface{}{
			"livenessProbeTimings":  map[string]interface{}{},
			"readinessProbeTimings": map[string]interface{}{},
			"startupProbeTimings":   map[string]interface{}{"failureThreshold": int64(10)},
		})).Spec.NodeSets[0]

		Expect(nodeSet.LivenessProbeTimings.Allow).Should(HaveValue(Equal([]string{"deadlocks"})))
		Expect(nodeSet.ReadinessProbeTimings.Deny).Should(HaveValue(Equal([]string{"stroom.security.common.impl.ExternalIdpConfigurationProvider"})))
		Expect(nodeSet.StartupProbeTimings.FailureThreshold).Should(Equal(int32(10)))
		Expect(nodeSet.StartupProbeTimings.LockWaitAllowanceSeconds).Should(Equal(int32(30)))
	})

	It("should not apply health check defaults when lists are explicitly empty", func() {
		nodeSet := createAndFetch(newCluster(map[string]interface{}{
			"livenessProbeTimings":  map[string]interface{}{"allow": []interface{}{}},
			"readinessProbeTimings": map[string]interface{}{"deny": []interface{}{}},
		})).Spec.NodeSets[0]

		Expect(nodeSet.LivenessProbeTimings.Allow).Should(HaveValue(BeEmpty()))
		Expect(nodeSet.ReadinessProbeTimings.Deny).Should(HaveValue(BeEmpty()))
	})

	It("should keep explicitly empty health check lists when the resource is updated", func() {
		// The controller updates the typed resource, for example to add a finalizer
		cluster := createAndFetch(newCluster(map[string]interface{}{
			"livenessProbeTimings":  map[string]interface{}{"allow": []interface{}{}},
			"readinessProbeTimings": map[string]interface{}{"deny": []interface{}{}},
		}))
		cluster.Finalizers = append(cluster.Finalizers, "stroom.gchq.github.io/test")
		Expect(k8sClient.Update(context.Background(), cluster)).To(Succeed())

		updated := &StroomCluster{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated)).To(Succeed())
		Expect(updated.Spec.NodeSets[0].LivenessProbeTimings.Allow).Should(HaveValue(BeEmpty()))
		Expect(updated.Spec.NodeSets[0].ReadinessProbeTimings.Deny).Should(HaveValue(BeEmpty()))

		updated.Finalizers = nil
		Expect(k8sClient.Update(context.Background(), updated)).To(Succeed())
	})

	It("should preserve user provided health check lists", func() {
		nodeSet := createAndFetch(newCluster(map[string]interface{}{
			"livenessProbeTimings":  map[string]interface{}{"allow": []interface{}{"deadlocks", "stroom.config.global.impl.AppConfigMonitor"}},
			"readinessProbeTimings": map[string]interface{}{"allow": []interface{}{"deadlocks"}, "deny": []interface{}{}},
		})).Spec.NodeSets[0]

		Expect(nodeSet.LivenessProbeTimings.Allow).Should(HaveValue(Equal([]string{"deadlocks", "stroom.config.global.impl.AppConfigMonitor"})))
		Expect(nodeSet.ReadinessProbeTimings.Allow).Should(HaveValue(Equal([]string{"deadlocks"})))
		Expect(nodeSet.ReadinessProbeTimings.Deny).Should(SatisfyAny(BeNil(), HaveValue(BeEmpty())))
	})

	It("should reject health check names containing a comma", func() {
		cluster := newCluster(map[string]interface{}{
			"livenessProbeTimings": map[string]interface{}{"allow": []interface{}{"deadlocks,other"}},
		})
		err := k8sClient.Create(context.Background(), cluster)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("allow[0]"))
	})

	It("should reject a negative lock wait allowance", func() {
		cluster := newCluster(map[string]interface{}{
			"startupProbeTimings": map[string]interface{}{"lockWaitAllowanceSeconds": int64(-1)},
		})
		Expect(k8sClient.Create(context.Background(), cluster)).ToNot(Succeed())
	})
})
