package v1

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("StroomCluster", func() {
	var key types.NamespacedName
	var created *StroomTaskAutoscaler

	BeforeEach(func() {
		key = types.NamespacedName{
			Name:      "stroom-" + rand.String(5),
			Namespace: "default",
		}
		created = &StroomTaskAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
			},
			Spec: StroomTaskAutoscalerSpec{
				StroomClusterRef: ResourceRef{
					Name:      "dev-cluster",
					Namespace: "default",
				},
				TaskName: "Data Processor",
			},
		}
	})

	AfterEach(func() {

	})

	Context("Create StroomTaskAutoscaler", func() {
		It("should create an object successfully", func() {
			By("creating an API object")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &StroomTaskAutoscaler{}
			Expect(k8sClient.Get(context.Background(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("having the correct default values")
			Expect(fetched.Spec.AdjustmentIntervalMins).To(Equal(1))

			By("deleting the created object")
			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), key, created)).ToNot(Succeed())
		})
	})
})
