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
			"nodeSets":          []interface{}{nodeSet},
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

	It("should not apply timing defaults when probe timings are omitted", func() {
		nodeSet := createAndFetch(newCluster(map[string]interface{}{})).Spec.NodeSets[0]

		Expect(nodeSet.StartupProbeTimings).Should(Equal(ProbeTimings{}))
		Expect(nodeSet.ReadinessProbeTimings).Should(Equal(ProbeTimings{}))
		Expect(nodeSet.LivenessProbeTimings).Should(Equal(ProbeTimings{}))
	})

	It("should apply timing defaults to empty probe timings", func() {
		nodeSet := createAndFetch(newCluster(map[string]interface{}{
			"startupProbeTimings":   map[string]interface{}{},
			"readinessProbeTimings": map[string]interface{}{},
			"livenessProbeTimings":  map[string]interface{}{},
		})).Spec.NodeSets[0]

		defaults := ProbeTimings{InitialDelaySeconds: 5, TimeoutSeconds: 5, PeriodSeconds: 5, SuccessThreshold: 1, FailureThreshold: 10}
		Expect(nodeSet.StartupProbeTimings).Should(Equal(defaults))
		Expect(nodeSet.ReadinessProbeTimings).Should(Equal(defaults))
		Expect(nodeSet.LivenessProbeTimings).Should(Equal(defaults))
	})

	It("should default the database names", func() {
		cluster := createAndFetch(newCluster(map[string]interface{}{}))
		Expect(cluster.Spec.AppDatabaseName).Should(Equal("stroom"))
		Expect(cluster.Spec.StatsDatabaseName).Should(Equal("stats"))
	})
})
