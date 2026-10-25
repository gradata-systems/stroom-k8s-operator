package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("DatabaseServer additional configuration", func() {

	create := func(lines ...string) error {
		dbServer := &DatabaseServer{
			ObjectMeta: metav1.ObjectMeta{Name: "db-" + rand.String(5), Namespace: "default"},
			Spec: DatabaseServerSpec{
				Image:            Image{Repository: "mysql"},
				DatabaseNames:    []string{"stroom"},
				AdditionalConfig: lines,
			},
		}
		err := k8sClient.Create(context.Background(), dbServer)
		if err == nil {
			Expect(k8sClient.Delete(context.Background(), dbServer)).To(Succeed())
		}
		return err
	}

	It("should accept server options", func() {
		Expect(create("max_connections=500", "innodb_buffer_pool_size = 1G", "skip-name-resolve")).To(Succeed())
	})

	for _, line := range []string{
		"[client]",
		"max_connections=500\n[client]",
		"# comment",
		"secure_file_priv=",
		"secure-file-priv = /",
		"LOCAL_INFILE=1",
		"init_file=/tmp/init.sql",
		"skip-grant-tables",
		"datadir=/tmp",
		" user=root",
	} {
		It("should reject the line '"+line+"'", func() {
			Expect(create(line)).ToNot(Succeed())
		})
	}
})
