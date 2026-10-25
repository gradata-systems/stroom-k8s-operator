package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	metrics "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

var _ = Describe("StroomTaskAutoscaler reconciler", func() {

	const namespace = "stroom"
	var (
		ctx     = context.Background()
		cluster *stroomv1.StroomCluster
	)

	// newReconciler returns a reconciler for the cluster, with a pod in its node set and metrics for that pod
	newReconciler := func(objects ...client.Object) *StroomTaskAutoscalerReconciler {
		testScheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(testScheme)).To(Succeed())
		Expect(stroomv1.AddToScheme(testScheme)).To(Succeed())
		Expect(metrics.AddToScheme(testScheme)).To(Succeed())

		nodeSet := &cluster.Spec.NodeSets[0]
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "stroom-dev-node-data-0", Namespace: namespace, Labels: cluster.GetNodeSetSelectorLabels(nodeSet),
		}}
		podMetrics := &metrics.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: namespace},
			Timestamp:  metav1.NewTime(time.Now()),
			Containers: []metrics.ContainerMetrics{{
				Name:  StroomNodeContainerName,
				Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			}},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(testScheme).
			WithObjects(append([]client.Object{cluster, pod, podMetrics}, objects...)...).Build()
		return &StroomTaskAutoscalerReconciler{Client: fakeClient, Scheme: testScheme, Metrics: NewNodeMetricMap()}
	}

	newAutoscaler := func(name string) *stroomv1.StroomTaskAutoscaler {
		return &stroomv1.StroomTaskAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: stroomv1.StroomTaskAutoscalerSpec{
				StroomClusterRef:         stroomv1.ResourceRef{Name: cluster.Name},
				TaskName:                 "Data Processor",
				AdjustmentIntervalMins:   1,
				MetricsSlidingWindowMins: 1,
				MinCpuPercent:            10,
				MaxCpuPercent:            90,
				MinTaskLimit:             1,
				MaxTaskLimit:             20,
				StepAmount:               1,
			},
		}
	}

	BeforeEach(func() {
		cluster = &stroomv1.StroomCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "dev", Namespace: namespace},
			Spec: stroomv1.StroomClusterSpec{NodeSets: []stroomv1.NodeSet{{
				Name: "data",
				Role: stroomv1.ProcessingNodeRole,
			}}},
		}
	})

	It("should keep each autoscaler's metrics separately", func() {
		reconciler := newReconciler(newAutoscaler("a"), newAutoscaler("b"))
		for _, name := range []string{"a", "b"} {
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}})
			Expect(err).ToNot(HaveOccurred())
		}

		Expect(reconciler.Metrics.Items).To(HaveLen(2))
		Expect(reconciler.Metrics.Items).To(HaveKeyWithValue("stroom/a|stroom/stroom-dev-node-data-0", HaveLen(1)))
		Expect(reconciler.Metrics.Items).To(HaveKeyWithValue("stroom/b|stroom/stroom-dev-node-data-0", HaveLen(1)))
	})

	It("should not scale a node set without a CPU limit", func() {
		reconciler := newReconciler()
		nodeSet := &cluster.Spec.NodeSets[0]
		pod := types.NamespacedName{Namespace: namespace, Name: "stroom-dev-node-data-0"}
		key := "stroom/a|" + pod.String()
		reconciler.Metrics.AddMetric(key, StroomNodeMetric{Time: time.Now(), CpuUsage: resource.NewMilliQuantity(500, resource.DecimalSI)})

		// The cluster has no database connection details, so this would fail if it tried to scale
		spec := newAutoscaler("a").Spec
		Expect(reconciler.scaleStroomNode(ctx, cluster, nodeSet, pod, key, &spec, time.Now())).To(Succeed())
	})
})
