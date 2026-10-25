package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

var _ = Describe("DatabaseBackup reconciler", func() {

	var (
		ctx        = context.Background()
		reconciler *DatabaseBackupReconciler
		dbBackup   *stroomv1.DatabaseBackup
		key        = types.NamespacedName{Namespace: "default", Name: "backup"}
	)

	BeforeEach(func() {
		testScheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(testScheme)).To(Succeed())
		Expect(stroomv1.AddToScheme(testScheme)).To(Succeed())

		dbBackup = &stroomv1.DatabaseBackup{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
			Spec: stroomv1.DatabaseBackupSpec{
				Image: stroomv1.Image{Repository: "mysql", Tag: "8.4"},
				DatabaseServerRef: stroomv1.DatabaseServerRef{
					ServerAddress: stroomv1.ServerAddress{Host: "mysql", Port: 3306, SecretName: "mysql"},
					UserName:      "stroomuser",
				},
				DatabaseNames: []string{"stroom"},
				Schedule:      "0 2 * * *",
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(dbBackup).Build()
		reconciler = &DatabaseBackupReconciler{Client: fakeClient, Scheme: testScheme}
	})

	reconcile := func() *batchv1.CronJob {
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).ToNot(HaveOccurred())
		cronJob := &batchv1.CronJob{}
		Expect(reconciler.Get(ctx, types.NamespacedName{Namespace: key.Namespace, Name: dbBackup.GetBaseName()}, cronJob)).To(Succeed())
		return cronJob
	}

	It("should create a batch/v1 CronJob", func() {
		cronJob := reconcile()
		Expect(cronJob.Spec.Schedule).To(Equal("0 2 * * *"))
		Expect(cronJob.OwnerReferences).To(ContainElement(HaveField("Name", key.Name)))
	})

	It("should update the CronJob when the DatabaseBackup changes", func() {
		reconcile()

		Expect(reconciler.Get(ctx, key, dbBackup)).To(Succeed())
		dbBackup.Spec.Schedule = "30 3 * * *"
		Expect(reconciler.Update(ctx, dbBackup)).To(Succeed())

		Expect(reconcile().Spec.Schedule).To(Equal("30 3 * * *"))
	})

	It("should do nothing when the DatabaseBackup no longer exists", func() {
		Expect(reconciler.Delete(ctx, dbBackup)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())
	})
})
