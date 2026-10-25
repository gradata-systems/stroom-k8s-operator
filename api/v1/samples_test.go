package v1

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/rand"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// The samples are documentation, so they must stay valid against the CRDs
var _ = Describe("Samples", func() {

	// decode returns the resources in a YAML stream
	decode := func(content io.Reader) []*unstructured.Unstructured {
		var documents []*unstructured.Unstructured
		decoder := utilyaml.NewYAMLOrJSONDecoder(content, 4096)
		for {
			document := &unstructured.Unstructured{}
			if err := decoder.Decode(&document.Object); errors.Is(err, io.EOF) {
				break
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			if len(document.Object) > 0 {
				documents = append(documents, document)
			}
		}
		return documents
	}

	// kustomize builds a kustomization with the kustomize binary the Makefile installs, or the one in KUSTOMIZE
	kustomize := func(dir string) []*unstructured.Unstructured {
		binary := os.Getenv("KUSTOMIZE")
		if binary == "" {
			binary = filepath.Join("..", "..", "bin", "kustomize")
		}
		_, err := os.Stat(binary)
		Expect(err).ToNot(HaveOccurred(), "kustomize is needed to build the samples: run make kustomize, or set KUSTOMIZE")

		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binary, "build", dir)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Run()).To(Succeed(), stderr.String())
		return decode(&stdout)
	}

	// create creates each resource in a new namespace, so the API server validates it, then deletes it
	create := func(documents []*unstructured.Unstructured) {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "samples-" + rand.String(5)}}
		Expect(k8sClient.Create(context.Background(), namespace)).To(Succeed())
		for _, document := range documents {
			document.SetNamespace(namespace.Name)
			Expect(k8sClient.Create(context.Background(), document)).To(Succeed(), "%v %v", document.GetKind(), document.GetName())
			Expect(k8sClient.Delete(context.Background(), document)).To(Succeed())
		}
	}

	find := func(documents []*unstructured.Unstructured, kind string, name string) *unstructured.Unstructured {
		for _, document := range documents {
			if document.GetKind() == kind && document.GetName() == name {
				return document
			}
		}
		Fail("no " + kind + " " + name)
		return nil
	}

	for _, deployment := range []string{"operator-database", "external-database"} {
		dir := filepath.Join("..", "..", "samples", deployment)

		It("should build the "+deployment+" sample, with only valid resources", func() {
			documents := kustomize(dir)
			for _, document := range documents {
				Expect(document.GetNamespace()).To(Equal("stroom"), "%v %v", document.GetKind(), document.GetName())
			}
			create(documents)
		})

		It("should configure the "+deployment+" sample's StroomCluster for the external identity provider", func() {
			documents := kustomize(dir)
			cluster := find(documents, "StroomCluster", "dev")

			configMapRef, _, err := unstructured.NestedStringMap(cluster.Object, "spec", "configMapRef")
			Expect(err).ToNot(HaveOccurred())
			Expect(configMapRef).To(Equal(map[string]string{"name": "stroom-config", "itemName": "config.yml"}))
			config, _, err := unstructured.NestedString(find(documents, "ConfigMap", "stroom-config").Object, "data", "config.yml")
			Expect(err).ToNot(HaveOccurred())
			Expect(config).To(ContainSubstring("identityProviderType: EXTERNAL_IDP"))

			clientSecret, _, err := unstructured.NestedString(cluster.Object, "spec", "openId", "clientSecret", "secretName")
			Expect(err).ToNot(HaveOccurred())
			Expect(clientSecret).To(Equal("stroom-operator-openid"))
			find(documents, "Secret", "stroom-operator-openid")
			find(documents, "Secret", "stroom-openid")

			// The rest of the spec is kept
			Expect(cluster.Object).To(HaveKeyWithValue("spec", HaveKey("nodeSets")))
			Expect(cluster.Object).To(HaveKeyWithValue("spec", HaveKey("https")))
		})
	}

	bundleFiles, err := filepath.Glob(filepath.Join("..", "..", "config", "samples", "stroom_v1_*.yaml"))
	Expect(err).ToNot(HaveOccurred())
	for _, file := range bundleFiles {
		It("should only contain valid resources in "+file, func() {
			content, err := os.Open(file)
			Expect(err).ToNot(HaveOccurred())
			defer content.Close()
			create(decode(content))
		})
	}
})
