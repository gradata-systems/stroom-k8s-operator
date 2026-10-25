package v1

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/rand"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// The samples are documentation, so they must stay valid against the CRDs
var _ = Describe("Samples", func() {

	files, err := filepath.Glob(filepath.Join("..", "..", "samples", "*.yaml"))
	Expect(err).ToNot(HaveOccurred())
	bundleFiles, err := filepath.Glob(filepath.Join("..", "..", "config", "samples", "stroom_v1_*.yaml"))
	Expect(err).ToNot(HaveOccurred())

	for _, file := range append(files, bundleFiles...) {
		It("should only contain valid resources in "+file, func() {
			content, err := os.Open(file)
			Expect(err).ToNot(HaveOccurred())
			defer content.Close()

			decoder := utilyaml.NewYAMLOrJSONDecoder(content, 4096)
			for {
				document := &unstructured.Unstructured{}
				if err := decoder.Decode(&document.Object); errors.Is(err, io.EOF) {
					break
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				if document.GroupVersionKind().Group != GroupVersion.Group {
					continue
				}

				// Create the resource under a unique name, so the API server validates it against its CRD
				document.SetNamespace("default")
				document.SetName(document.GetName() + "-" + rand.String(5))
				Expect(k8sClient.Create(context.Background(), document)).To(Succeed(), "%v %v", document.GetKind(), document.GetName())
				Expect(k8sClient.Delete(context.Background(), document)).To(Succeed())
			}
		})
	}
})
