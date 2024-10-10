package v1

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("StroomCluster", func() {
	BeforeEach(func() {

	})

	AfterEach(func() {

	})

	Context("Create StroomCluster", func() {
		It("should create an object successfully", func() {
			key := types.NamespacedName{
				Name:      "stroom-" + rand.String(5),
				Namespace: "default",
			}
			created := &StroomCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: StroomClusterSpec{
					Image: Image{
						Repository: "gchq/stroom",
					},
					DatabaseServerRef: DatabaseServerRef{},
					AppDatabaseName:   "stroom",
					StatsDatabaseName: "stats",
					Ingress:           IngressSettings{},
					NodeSets: []NodeSet{{
						Name:  "nodeset-1",
						Count: 1,
						MemoryOptions: JvmMemoryOptions{
							InitialPercentage: 50,
							MaxPercentage:     75,
						},
					}},
					LogSender: LogSenderSettings{
						Image: Image{
							Repository: "gchq/stroom-log-sender",
						},
					},
				},
			}

			By("creating an API object")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &StroomCluster{}
			Expect(k8sClient.Get(context.Background(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("deleting the created object")
			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), key, created)).ToNot(Succeed())
		})
	})
})
