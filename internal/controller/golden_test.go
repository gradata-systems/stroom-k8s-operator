package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

// The golden tests render every resource the controllers create for each fixture in testdata/fixtures, and compare
// the result with the files in testdata/golden. Any change to what the operator deploys shows up as a diff of those
// files. To accept a change, regenerate them with UPDATE_GOLDEN=1 and review the diff.
const (
	fixturesDir = "testdata/fixtures"
	goldenDir   = "testdata/golden"
)

var _ = Describe("Rendered resources", func() {

	fixtures, err := filepath.Glob(filepath.Join(fixturesDir, "*.yaml"))
	if err != nil || len(fixtures) == 0 {
		panic(fmt.Sprintf("no fixtures found in %v: %v", fixturesDir, err))
	}

	for _, fixture := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixture), ".yaml")

		It("should match the golden files for fixture '"+name+"'", func() {
			namespace := "golden-" + name
			objects := createFixture(fixture, namespace)
			rendered := renderFixture(objects)
			compareWithGoldenFiles(filepath.Join(goldenDir, name), rendered)
		})
	}
})

// createFixture creates each resource in the fixture file via the API server, so CRD defaults are applied, and
// returns the resources as stored
func createFixture(path string, namespace string) []client.Object {
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	Expect(client.IgnoreAlreadyExists(apiClient.Create(ctx, ns))).To(Succeed())

	file, err := os.Open(path)
	Expect(err).ToNot(HaveOccurred())
	defer file.Close()

	var objects []client.Object
	decoder := utilyaml.NewYAMLOrJSONDecoder(file, 4096)
	for {
		document := &unstructured.Unstructured{}
		if err := decoder.Decode(&document.Object); errors.Is(err, io.EOF) {
			break
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
		if len(document.Object) == 0 {
			continue
		}

		// Create the resource as written, as kubectl would. Creating a typed object would send empty values for
		// fields without omitempty, which prevents their defaults from being applied.
		document.SetNamespace(namespace)
		Expect(client.IgnoreAlreadyExists(apiClient.Create(ctx, document))).To(Succeed())

		typed, err := scheme.Scheme.New(document.GroupVersionKind())
		Expect(err).ToNot(HaveOccurred())
		object := typed.(client.Object)
		Expect(apiClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: document.GetName()}, object)).To(Succeed())
		objects = append(objects, object)
	}
	return objects
}

// renderFixture renders the resources each controller creates for the fixture resources, keyed by file name
func renderFixture(objects []client.Object) map[string]client.Object {
	rendered := map[string]client.Object{}
	add := func(object client.Object) {
		kind := strings.ToLower(fmt.Sprintf("%T", object)[strings.LastIndex(fmt.Sprintf("%T", object), ".")+1:])
		rendered[fmt.Sprintf("%v-%v.yaml", kind, object.GetName())] = object
	}

	dbServerReconciler := &DatabaseServerReconciler{Client: apiClient, Scheme: scheme.Scheme}
	dbBackupReconciler := &DatabaseBackupReconciler{Client: apiClient, Scheme: scheme.Scheme}
	stroomClusterReconciler := &StroomClusterReconciler{Client: apiClient, Scheme: scheme.Scheme}

	for _, object := range objects {
		switch resource := object.(type) {
		case *stroomv1.DatabaseServer:
			add(dbServerReconciler.createSecret(resource))
			add(dbServerReconciler.createConfigMap(resource))
			add(dbServerReconciler.createDbInitConfigMap(resource))
			add(dbServerReconciler.createStatefulSet(resource))
			add(dbServerReconciler.createService(resource))

		case *stroomv1.DatabaseBackup:
			add(dbBackupReconciler.createCronJob(resource, goldenDatabaseConnectionInfo(resource.Name)))

		case *stroomv1.StroomCluster:
			dbInfo := goldenDatabaseConnectionInfo(resource.Spec.DatabaseServerRef.ServerRef.Name)
			add(stroomClusterReconciler.createServiceAccount(resource))
			add(stroomClusterReconciler.createConfigMap(resource, readStaticFiles()))
			if !resource.Spec.LogSender.IsZero() {
				add(stroomClusterReconciler.createLogSenderConfigMap(resource))
			}
			for i := range resource.Spec.NodeSets {
				nodeSet := &resource.Spec.NodeSets[i]
				add(stroomClusterReconciler.createStatefulSet(resource, nodeSet, dbInfo))
				add(stroomClusterReconciler.createService(resource, nodeSet, resource.GetNodeSetHeadlessServiceName(nodeSet), corev1.ClusterIPNone))
				add(stroomClusterReconciler.createService(resource, nodeSet, resource.GetNodeSetServiceName(nodeSet), ""))
			}
			for _, ingress := range stroomClusterReconciler.createIngresses(context.Background(), resource) {
				add(&ingress)
			}
		}
	}
	return rendered
}

// goldenDatabaseConnectionInfo returns fixed database connection details, as resolved by the controllers
func goldenDatabaseConnectionInfo(dbServerName string) *DatabaseConnectionInfo {
	return &DatabaseConnectionInfo{
		ServerAddress: stroomv1.ServerAddress{
			Host:       fmt.Sprintf("stroom-%v-db", dbServerName),
			Port:       3306,
			SecretName: fmt.Sprintf("stroom-%v-db", dbServerName),
		},
		UserName: DatabaseServiceUserName,
	}
}

// readStaticFiles returns the static content files, as the StroomCluster controller provides them in a ConfigMap
func readStaticFiles() map[string]string {
	entries, err := StaticFiles.ReadDir("static_content")
	Expect(err).ToNot(HaveOccurred())

	files := map[string]string{}
	for _, entry := range entries {
		data, err := StaticFiles.ReadFile("static_content/" + entry.Name())
		Expect(err).ToNot(HaveOccurred())
		files[entry.Name()] = string(data)
	}
	return files
}

// normalise replaces values that differ between test runs, so the rendered output is stable
func normalise(object client.Object) client.Object {
	object = object.DeepCopyObject().(client.Object)
	ownerReferences := object.GetOwnerReferences()
	for i := range ownerReferences {
		ownerReferences[i].UID = "<uid>"
	}
	object.SetOwnerReferences(ownerReferences)

	switch resource := object.(type) {
	case *corev1.Secret:
		// Passwords are randomly generated
		for key := range resource.Data {
			resource.Data[key] = []byte("<generated>")
		}
	case *corev1.ConfigMap:
		// Static content is checked by file hash, as the scripts are tested separately
		if strings.HasSuffix(resource.Name, "-static-content") {
			for key, value := range resource.Data {
				resource.Data[key] = fmt.Sprintf("<sha256:%x>", sha256.Sum256([]byte(value)))
			}
		}
	}
	return object
}

func compareWithGoldenFiles(dir string, rendered map[string]client.Object) {
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	if update {
		Expect(os.RemoveAll(dir)).To(Succeed())
		Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
	}

	var renderedNames []string
	for fileName, object := range rendered {
		renderedNames = append(renderedNames, fileName)
		actual, err := yaml.Marshal(normalise(object))
		Expect(err).ToNot(HaveOccurred())

		path := filepath.Join(dir, fileName)
		if update {
			Expect(os.WriteFile(path, actual, 0o644)).To(Succeed())
			continue
		}
		expected, err := os.ReadFile(path)
		Expect(err).ToNot(HaveOccurred(), "missing golden file %v: regenerate with UPDATE_GOLDEN=1", path)
		Expect(string(actual)).To(Equal(string(bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n")))),
			"%v differs from the rendered resource: regenerate with UPDATE_GOLDEN=1 if the change is intended", path)
	}

	// Golden files for resources that are no longer rendered must be removed
	goldenFiles, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	Expect(err).ToNot(HaveOccurred())
	var goldenNames []string
	for _, goldenFile := range goldenFiles {
		goldenNames = append(goldenNames, filepath.Base(goldenFile))
	}
	sort.Strings(renderedNames)
	sort.Strings(goldenNames)
	Expect(goldenNames).To(Equal(renderedNames), "golden files in %v don't match the rendered resources", dir)
}
