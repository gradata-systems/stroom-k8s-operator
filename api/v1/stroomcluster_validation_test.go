package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("StroomCluster validation", func() {

	newCluster := func(spec map[string]interface{}, nodeSet map[string]interface{}) *unstructured.Unstructured {
		nodeSet["name"] = "node"
		nodeSet["count"] = int64(1)
		nodeSet["localDataVolumeClaim"] = map[string]interface{}{}
		nodeSet["resources"] = map[string]interface{}{}
		nodeSet["memoryOptions"] = map[string]interface{}{"initialPercentage": int64(50), "maxPercentage": int64(75)}
		spec["image"] = map[string]interface{}{"repository": "gchq/stroom"}
		spec["databaseServerRef"] = map[string]interface{}{}
		spec["openId"] = map[string]interface{}{"clientId": "stroom", "clientSecret": map[string]interface{}{"secretName": "openid", "key": "secret"}}
		spec["ingress"] = map[string]interface{}{"hostName": "stroom.example.com"}
		spec["nodeSets"] = []interface{}{nodeSet}

		cluster := &unstructured.Unstructured{}
		cluster.SetAPIVersion(GroupVersion.String())
		cluster.SetKind("StroomCluster")
		cluster.SetName("stroom-" + rand.String(5))
		cluster.SetNamespace("default")
		cluster.Object["spec"] = spec
		return cluster
	}

	It("should accept each node role", func() {
		for _, role := range []string{"Processing", "Frontend"} {
			cluster := newCluster(map[string]interface{}{}, map[string]interface{}{"role": role})
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed(), role)
			Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())
		}
	})

	It("should reject an unknown node role", func() {
		cluster := newCluster(map[string]interface{}{}, map[string]interface{}{"role": "Frontent"})
		Expect(k8sClient.Create(context.Background(), cluster)).ToNot(Succeed())
	})

	It("should accept each volume claim delete policy", func() {
		for _, policy := range []string{"DeleteOnScaledownAndClusterDeletion", "DeleteOnScaledownOnly"} {
			cluster := newCluster(map[string]interface{}{"volumeClaimDeletePolicy": policy}, map[string]interface{}{})
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed(), policy)
			Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())
		}
	})

	It("should reject an unknown volume claim delete policy", func() {
		cluster := newCluster(map[string]interface{}{"volumeClaimDeletePolicy": "DeleteAlways"}, map[string]interface{}{})
		Expect(k8sClient.Create(context.Background(), cluster)).ToNot(Succeed())
	})
})
