package controllers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

var _ = Describe("StroomTaskAutoscaler utilities", func() {

	var (
		currentTime   = time.Date(2021, 1, 1, 1, 25, 0, 0, time.UTC)
		podName       = "pod-1"
		_podMetricMap = NodeMetricMap{Items: map[string][]NodeMetric{
			podName: {
				{Time: time.Date(2021, 1, 1, 1, 0, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 1, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 2, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 3, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 10, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 20, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(10, resource.DecimalExponent)},
			},
		}}

		podMetricMap NodeMetricMap
	)

	BeforeEach(func() {
		podMetricMap = NewNodeMetricMap()
		for key, value := range _podMetricMap.Items {
			podMetricMap.Items[key] = make([]NodeMetric, len(value))
			for i, metric := range value {
				podMetricMap.Items[key][i] = metric
			}
		}
	})

	Context("NodeMetricMap AgeOff()", func() {
		It("Should have one item left with a 10-minute retention period", func() {
			podMetricMap.AgeOff(10, currentTime)
			Expect(len(podMetricMap.Items[podName])).Should(Equal(1))
		})
		It("Should have two items left with a 20-minute retention period", func() {
			podMetricMap.AgeOff(20, currentTime)
			Expect(len(podMetricMap.Items[podName])).Should(Equal(2))
		})
		It("Should have six items left with a 30-minute retention period", func() {
			podMetricMap.AgeOff(30, currentTime)
			Expect(len(podMetricMap.Items[podName])).Should(Equal(6))
		})
	})

	Context("NodeMetricMap GetSlidingWindowMean()", func() {
		It("Should calculate the mean correctly given an interval of 20 minutes", func() {
			var mean int64
			podMetricMap.GetSlidingWindowMean(podName, 20, currentTime, &mean)
			Expect(mean).Should(Equal(int64((2000 + 10000) / 2)))
		})
		It("Should calculate the mean correctly given an interval of 30 minutes", func() {
			var mean int64
			podMetricMap.GetSlidingWindowMean(podName, 30, currentTime, &mean)
			Expect(mean).Should(Equal(int64((2000*5 + 10000) / 6)))
		})
	})
})
