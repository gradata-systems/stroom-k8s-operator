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

	for _, name := range []string{"x; DROP DATABASE mysql", "x`y", "$(id)", "a b", "", "a2345678901234567890123456789012345678901234567890123456789012345"} {
		It("should reject the database name '"+name+"'", func() {
			Expect(k8sClient.Create(context.Background(), newDatabaseServer(name))).ToNot(Succeed())
		})
	}

	It("should reject an invalid database name in a DatabaseBackup", func() {
		dbBackup := &DatabaseBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "backup-" + rand.String(5), Namespace: "default"},
			Spec: DatabaseBackupSpec{
				Image:         Image{Repository: "mysql"},
				DatabaseNames: []string{"stroom $(id)"},
				Schedule:      "0 0 * * *",
			},
		}
		Expect(k8sClient.Create(context.Background(), dbBackup)).ToNot(Succeed())
	})
})
