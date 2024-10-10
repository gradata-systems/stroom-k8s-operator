package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

var _ = Describe("StroomTaskAutoscaler utilities", func() {

	var (
		currentTime   = time.Date(2021, 1, 1, 1, 25, 0, 0, time.UTC)
		podName       = "pod-1"
		_podMetricMap = StroomNodeMetricMap{Items: map[string][]StroomNodeMetric{
			podName: {
				{Time: time.Date(2021, 1, 1, 1, 0, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 1, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 2, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 3, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 10, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(2, resource.DecimalExponent)},
				{Time: time.Date(2021, 1, 1, 1, 20, 0, 0, time.UTC), CpuUsage: resource.NewQuantity(10, resource.DecimalExponent)},
			},
		}}

		nodeMetricMap          StroomNodeMetricMap
		adjustmentIntervalMins = 5
	)

	BeforeEach(func() {
		nodeMetricMap = NewNodeMetricMap()
		for key, value := range _podMetricMap.Items {
			nodeMetricMap.Items[key] = make([]StroomNodeMetric, len(value))
			for i, metric := range value {
				nodeMetricMap.Items[key][i] = metric
			}
		}
	})

	Context("StroomNodeMetricMap AgeOff()", func() {
		It("should have one item left with a 10-minute retention period", func() {
			nodeMetricMap.AgeOff(10, currentTime)
			Expect(len(nodeMetricMap.Items[podName])).Should(Equal(1))
		})
		It("should have two items left with a 20-minute retention period", func() {
			nodeMetricMap.AgeOff(20, currentTime)
			Expect(len(nodeMetricMap.Items[podName])).Should(Equal(2))
		})
		It("should have six items left with a 30-minute retention period", func() {
			nodeMetricMap.AgeOff(30, currentTime)
			Expect(len(nodeMetricMap.Items[podName])).Should(Equal(6))
		})
	})

	Context("StroomNodeMetricMap GetSlidingWindowMean()", func() {
		It("should calculate the mean correctly given an interval of 20 minutes", func() {
			var mean int64
			nodeMetricMap.GetSlidingWindowMean(podName, 20, currentTime, &mean)
			Expect(mean).Should(Equal(int64((2000 + 10000) / 2)))
		})
		It("should calculate the mean correctly given an interval of 30 minutes", func() {
			var mean int64
			nodeMetricMap.GetSlidingWindowMean(podName, 30, currentTime, &mean)
			Expect(mean).Should(Equal(int64((2000*5 + 10000) / 6)))
		})
	})

	Context("StroomNodeMetricMap scaling", func() {
		It("should store the last scaled time", func() {
			nodeMetricMap.SetLastScaled(podName, currentTime)
			Expect(nodeMetricMap.LastScaled[podName]).To(Equal(currentTime))
		})
		It("should be scaled when the current time has exceeded the adjustment interval", func() {
			nodeMetricMap.SetLastScaled(podName, currentTime)
			currentTime = currentTime.Add(time.Minute * 6)
			Expect(nodeMetricMap.ShouldScale(podName, adjustmentIntervalMins, currentTime)).To(BeTrue())
		})
	})
})
