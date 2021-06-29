package v1

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

var _ = Describe("DatabaseServer", func() {
	var (
		key              types.NamespacedName
		created, fetched *DatabaseServer
	)

	BeforeEach(func() {

	})

	AfterEach(func() {

	})

	Context("Create API", func() {
		It("should create an object successfully", func() {
			key = types.NamespacedName{
				Namespace: "default",
				Name:      "db-" + rand.String(5),
			}
			created = &DatabaseServer{
				ObjectMeta: v1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
			}
			By("creating an API obj")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched = &DatabaseServer{}
			Expect(k8sClient.Get(context.Background(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("deleting the created object")
			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), key, created)).ToNot(Succeed())
		})
		It("should correctly handle finalizers", func() {
			dbServer := &DatabaseServer{
				ObjectMeta: v1.ObjectMeta{
					DeletionTimestamp: &v1.Time{
						Time: time.Now(),
					},
				},
			}
			Expect(dbServer.IsBeingDeleted()).To(BeTrue())

			controllerutil.AddFinalizer(dbServer, StroomClusterFinalizerName)
			Expect(len(dbServer.GetFinalizers())).To(Equal(1))
			Expect(controllerutil.ContainsFinalizer(dbServer, StroomClusterFinalizerName)).To(BeTrue())

			controllerutil.RemoveFinalizer(dbServer, StroomClusterFinalizerName)
			Expect(len(dbServer.GetFinalizers())).To(Equal(0))
			Expect(controllerutil.ContainsFinalizer(dbServer, StroomClusterFinalizerName)).To(BeFalse())
		})
	})
})
