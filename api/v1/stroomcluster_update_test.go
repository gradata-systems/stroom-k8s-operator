package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

// The operator updates the StroomCluster resources it reads, for example to add a finalizer, so a resource created
// with only its required fields must survive a typed update
var _ = Describe("StroomCluster updates", func() {

	It("should reject a typed update of a StroomCluster without a log sender", func() {
		// BASELINE: known bug. The empty log sender settings are sent with an empty image repository, which fails
		// validation, so the operator can't add its finalizer to such a StroomCluster.
		cluster := &unstructured.Unstructured{}
		cluster.SetAPIVersion(GroupVersion.String())
		cluster.SetKind("StroomCluster")
		cluster.SetName("stroom-" + rand.String(5))
		cluster.SetNamespace("default")
		cluster.Object["spec"] = map[string]interface{}{
			"image":             map[string]interface{}{"repository": "gchq/stroom"},
			"databaseServerRef": map[string]interface{}{},
			"openId":            map[string]interface{}{"clientId": "stroom", "clientSecret": map[string]interface{}{"secretName": "openid", "key": "secret"}},
			"ingress":           map[string]interface{}{"hostName": "stroom.example.com"},
			"nodeSets": []interface{}{map[string]interface{}{
				"name":                 "node",
				"count":                int64(1),
				"localDataVolumeClaim": map[string]interface{}{},
				"resources":            map[string]interface{}{},
				"memoryOptions":        map[string]interface{}{"initialPercentage": int64(50), "maxPercentage": int64(75)},
			}},
		}
		Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())
		})

		fetched := &StroomCluster{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.GetName(), Namespace: cluster.GetNamespace()}, fetched)).To(Succeed())
		fetched.Labels = map[string]string{"updated": "true"}
		err := k8sClient.Update(context.Background(), fetched)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.logSender.image.repository"))
	})
})
