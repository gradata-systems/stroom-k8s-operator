package controller

import (
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

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
})
