package controllers

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

const (
	MaximumMetricRetentionPeriodMins = 60
)

type NodeMetric struct {
	Time     time.Time
	CpuUsage *resource.Quantity
}

func (in *NodeMetric) IsZero() bool {
	return in.Time.IsZero() && in.CpuUsage == nil
}

type NodeMetricMap struct {
	// Stored pod metrics
	Items map[string][]NodeMetric

	// When a pod last had its tasks autoscaled
	LastScaled map[string]time.Time
}

func NewNodeMetricMap() NodeMetricMap {
	return NodeMetricMap{
		Items:      map[string][]NodeMetric{},
		LastScaled: map[string]time.Time{},
	}
}

func (in *NodeMetricMap) AddMetric(podName string, metric NodeMetric) {
	if metrics, exists := in.Items[podName]; exists {
		in.Items[podName] = append(metrics, metric)
	} else {
		in.Items[podName] = []NodeMetric{metric}
	}
}

func (in *NodeMetricMap) SetLastScaled(podName string, currentTime time.Time) {
	if _, exists := in.LastScaled[podName]; !exists {
		in.LastScaled[podName] = currentTime
	} else {
		in.LastScaled[podName] = currentTime
	}
}

func (in *NodeMetricMap) ShouldScale(podName string, adjustmentIntervalMins int, currentTime time.Time) bool {
	if lastScaled, exists := in.LastScaled[podName]; exists {
		return lastScaled.Add(time.Minute * time.Duration(adjustmentIntervalMins)).After(currentTime)
	}

	return false
}

// AgeOff removes metrics in the map older than the specified retention period (in minutes)
func (in *NodeMetricMap) AgeOff(retentionPeriodMins int, currentTime time.Time) {
	for podName, metricList := range in.Items {
		newMetricList := make([]NodeMetric, 0)

		for _, metric := range metricList {
			// If the metric is within the retention period, keep it
			if metric.Time.After(currentTime.Add(time.Minute * time.Duration(-retentionPeriodMins))) {
				newMetricList = append(newMetricList, metric)
			}
		}

		in.Items[podName] = newMetricList
	}
}

// GetSlidingWindowMean calculates the mean of all metrics for a pod name, within the specified sliding window interval (in minutes)
func (in *NodeMetricMap) GetSlidingWindowMean(podName string, slidingWindowIntervalMins int, currentTime time.Time, result *int64) bool {
	if metrics, exists := in.Items[podName]; exists {
		var sum int64 = 0
		var valueCount int64 = 0
		for _, metric := range metrics {
			// Check if metric is within the statistic window
			if metric.Time.After(currentTime.Add(time.Minute * time.Duration(-slidingWindowIntervalMins))) {
				sum += metric.CpuUsage.MilliValue()
				valueCount++
			}
		}

		*result = sum / valueCount
		return true
	}

	return false
}
