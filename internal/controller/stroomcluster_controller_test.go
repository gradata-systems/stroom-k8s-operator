package controller

import (
	"context"
	"fmt"
	"regexp"

	"github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

var _ = Describe("StroomCluster database server claims", func() {

	var (
		ctx        = context.Background()
		reconciler *StroomClusterReconciler
		cluster    *stroomv1.StroomCluster
		dbServer   *stroomv1.DatabaseServer
	)

	// newReconciler returns a reconciler whose client holds the given resources
	newReconciler := func(objects ...client.Object) *StroomClusterReconciler {
		testScheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(testScheme)).To(Succeed())
		Expect(stroomv1.AddToScheme(testScheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(objects...).Build()
		return &StroomClusterReconciler{Client: fakeClient, Scheme: testScheme}
	}

	// claim claims the database server for the cluster, returning the database server's claim afterwards
	claim := func() (stroomv1.ResourceRef, error) {
		err := reconciler.claimDatabaseServer(ctx, cluster, stroomv1.ResourceRef{Name: dbServer.Name, Namespace: dbServer.Namespace}, dbServer)
		stored := &stroomv1.DatabaseServer{}
		Expect(reconciler.Get(ctx, types.NamespacedName{Name: dbServer.Name, Namespace: dbServer.Namespace}, stored)).To(Succeed())
		return stored.StroomClusterRef, err
	}

	BeforeEach(func() {
		cluster = &stroomv1.StroomCluster{ObjectMeta: metav1.ObjectMeta{Name: "dev", Namespace: "stroom"}}
		// The DatabaseServer's name differs from the StroomCluster's
		dbServer = &stroomv1.DatabaseServer{ObjectMeta: metav1.ObjectMeta{Name: "mysql", Namespace: "stroom"}}
	})

	It("should record the claiming StroomCluster", func() {
		reconciler = newReconciler(cluster, dbServer)
		ref, err := claim()
		Expect(err).ToNot(HaveOccurred())
		Expect(ref).To(Equal(stroomv1.ResourceRef{Name: "dev", Namespace: "stroom"}))

		// Claiming again is allowed
		ref, err = claim()
		Expect(err).ToNot(HaveOccurred())
		Expect(ref).To(Equal(stroomv1.ResourceRef{Name: "dev", Namespace: "stroom"}))
	})

	It("should replace a claim recording the DatabaseServer's own name, as earlier versions did", func() {
		dbServer.StroomClusterRef = stroomv1.ResourceRef{Name: "mysql", Namespace: "stroom"}
		reconciler = newReconciler(cluster, dbServer)
		ref, err := claim()
		Expect(err).ToNot(HaveOccurred())
		Expect(ref).To(Equal(stroomv1.ResourceRef{Name: "dev", Namespace: "stroom"}))
	})

	It("should refuse a DatabaseServer claimed by another StroomCluster", func() {
		other := &stroomv1.StroomCluster{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "stroom"}}
		dbServer.StroomClusterRef = stroomv1.ResourceRef{Name: "other", Namespace: "stroom"}
		reconciler = newReconciler(cluster, other, dbServer)
		ref, err := claim()
		Expect(err).To(HaveOccurred())
		Expect(ref).To(Equal(stroomv1.ResourceRef{Name: "other", Namespace: "stroom"}))
	})
})

var _ = Describe("StroomCluster deletion", func() {

	Context("clusterNodeNamePattern()", func() {
		cluster := &stroomv1.StroomCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "dev"},
			Spec: stroomv1.StroomClusterSpec{NodeSets: []stroomv1.NodeSet{
				{Name: "data"},
				{Name: "ui"},
			}},
		}
		pattern := regexp.MustCompile(clusterNodeNamePattern(cluster))

		It("should match the cluster's nodes", func() {
			for _, node := range []string{"stroom-dev-node-data-0", "stroom-dev-node-data-12", "stroom-dev-node-ui-0"} {
				Expect(pattern.MatchString(node)).To(BeTrue(), node)
			}
		})
		It("should not match the nodes of other clusters whose names start the same way", func() {
			for _, node := range []string{
				"stroom-dev2-node-data-0",
				"stroom-dev-node-data-node-x-0",
				"stroom-dev-node-database-0",
				"stroom-dev-node-data-0-extra",
				"stroom-dev-node-data_0",
			} {
				Expect(pattern.MatchString(node)).To(BeFalse(), node)
			}
		})
	})

	Context("isMissingTable()", func() {
		It("should recognise a missing table, as in a database no Stroom node has started with", func() {
			err := &mysql.MySQLError{Number: 1146, Message: "Table 'stroom.job_node' doesn't exist"}
			Expect(isMissingTable(err)).To(BeTrue())
			Expect(isMissingTable(fmt.Errorf("disabling jobs: %w", err))).To(BeTrue())
		})
		It("should not treat other errors as a missing table", func() {
			Expect(isMissingTable(nil)).To(BeFalse())
			Expect(isMissingTable(&mysql.MySQLError{Number: 1045, Message: "Access denied"})).To(BeFalse())
			Expect(isMissingTable(fmt.Errorf("connection refused"))).To(BeFalse())
		})
	})
})
