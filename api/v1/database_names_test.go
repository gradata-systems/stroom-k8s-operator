package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("Database names", func() {

	newDatabaseServer := func(databaseNames ...string) *DatabaseServer {
		return &DatabaseServer{
			ObjectMeta: metav1.ObjectMeta{Name: "db-" + rand.String(5), Namespace: "default"},
			Spec: DatabaseServerSpec{
				Image:         Image{Repository: "mysql"},
				DatabaseNames: databaseNames,
			},
		}
	}

	It("should accept valid database names", func() {
		dbServer := newDatabaseServer("stroom", "stats_2")
		Expect(k8sClient.Create(context.Background(), dbServer)).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), dbServer)).To(Succeed())
	})

	It("should accept any database name", func() {
		// BASELINE: known issue, see security review H1/H2. Names are not validated, and are later interpolated into
		// SQL and shell commands.
		dbServer := newDatabaseServer("x; DROP DATABASE mysql", "$(id)")
		Expect(k8sClient.Create(context.Background(), dbServer)).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), dbServer)).To(Succeed())
	})
})
