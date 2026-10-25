package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

var _ = Describe("DatabaseServer reconciler", func() {

	var (
		ctx        = context.Background()
		reconciler *DatabaseServerReconciler
		dbServer   *stroomv1.DatabaseServer
		key        = types.NamespacedName{Namespace: "stroom", Name: "db"}
	)

	BeforeEach(func() {
		testScheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(testScheme)).To(Succeed())
		Expect(stroomv1.AddToScheme(testScheme)).To(Succeed())

		dbServer = &stroomv1.DatabaseServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:        key.Name,
				Namespace:   key.Namespace,
				Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
			},
			Spec: stroomv1.DatabaseServerSpec{
				Image:          stroomv1.Image{Repository: "mysql", Tag: "8.4"},
				DatabaseNames:  []string{"stroom"},
				PodAnnotations: map[string]string{"example.com/annotation": "pod"},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(testScheme).
			WithObjects(dbServer).WithStatusSubresource(dbServer).Build()
		reconciler = &DatabaseServerReconciler{Client: fakeClient, Scheme: testScheme}
	})

	// reconcile reconciles until the DatabaseServer's resources are created, returning its StatefulSet
	reconcile := func() *appsv1.StatefulSet {
		for range 5 {
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).ToNot(HaveOccurred())
			if result.IsZero() {
				break
			}
		}
		statefulSet := &appsv1.StatefulSet{}
		Expect(reconciler.Get(ctx, types.NamespacedName{Namespace: key.Namespace, Name: dbServer.GetBaseName()}, statefulSet)).To(Succeed())
		return statefulSet
	}

	update := func(change func(*stroomv1.DatabaseServer)) {
		Expect(reconciler.Get(ctx, key, dbServer)).To(Succeed())
		change(dbServer)
		Expect(reconciler.Update(ctx, dbServer)).To(Succeed())
	}

	It("should annotate the MySQL pod with the pod annotations, not the DatabaseServer's own", func() {
		annotations := reconcile().Spec.Template.Annotations
		Expect(annotations).To(Equal(map[string]string{"example.com/annotation": "pod"}))
	})

	It("should let StroomClusters tell when the DatabaseServer is being updated", func() {
		clusterReconciler := &StroomClusterReconciler{Client: reconciler.Client, Scheme: reconciler.Scheme}
		settled := func() (bool, string) {
			Expect(reconciler.Get(ctx, key, dbServer)).To(Succeed())
			settled, reason, err := clusterReconciler.databaseServerSettled(ctx, dbServer)
			Expect(err).ToNot(HaveOccurred())
			return settled, reason
		}
		// setRolledOut sets the StatefulSet's status as the StatefulSet controller would once its pod is updated and ready
		setRolledOut := func() {
			statefulSet := reconcile()
			statefulSet.Status = appsv1.StatefulSetStatus{
				ObservedGeneration: statefulSet.Generation, Replicas: 1, ReadyReplicas: 1, UpdatedReplicas: 1,
				CurrentRevision: "rev", UpdateRevision: "rev",
			}
			Expect(reconciler.Status().Update(ctx, statefulSet)).To(Succeed())
		}

		ok, reason := settled()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("its StatefulSet doesn't exist yet"))

		statefulSet := reconcile()
		Expect(statefulSet.Annotations).To(HaveKeyWithValue(DatabaseServerTemplateHashAnnotation, podTemplateHash(&statefulSet.Spec.Template)))
		ok, reason = settled()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("its pods are being updated"))

		setRolledOut()
		Expect(settled()).To(BeTrue())

		By("changing the DatabaseServer, before its reconciler has updated the StatefulSet")
		update(func(db *stroomv1.DatabaseServer) { db.Spec.Image.Tag = "8.4.6" })
		ok, reason = settled()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("its StatefulSet is about to be updated"))

		setRolledOut()
		Expect(settled()).To(BeTrue())
	})

	It("should update the MySQL pod template when the DatabaseServer changes", func() {
		reconcile()
		update(func(db *stroomv1.DatabaseServer) { db.Spec.Image.Tag = "8.4.6" })
		Expect(reconcile().Spec.Template.Spec.Containers[0].Image).To(Equal("mysql:8.4.6"))
	})

	It("should update the MySQL configuration when the DatabaseServer changes", func() {
		reconcile()
		update(func(db *stroomv1.DatabaseServer) { db.Spec.AdditionalConfig = []string{"max_connections=500"} })
		reconcile()

		configMap := &corev1.ConfigMap{}
		Expect(reconciler.Get(ctx, types.NamespacedName{Namespace: key.Namespace, Name: dbServer.GetConfigMapName()}, configMap)).To(Succeed())
		Expect(configMap.Data["my.cnf"]).To(ContainSubstring("max_connections=500"))
	})

	It("should keep the generated passwords", func() {
		reconcile()
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{Namespace: key.Namespace, Name: dbServer.GetSecretName()}
		Expect(reconciler.Get(ctx, secretKey, secret)).To(Succeed())
		passwords := secret.Data

		update(func(db *stroomv1.DatabaseServer) { db.Spec.Image.Tag = "8.4.6" })
		reconcile()
		Expect(reconciler.Get(ctx, secretKey, secret)).To(Succeed())
		Expect(secret.Data).To(Equal(passwords))
	})
})
