package controllers

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

const (
	MaximumMetricRetentionPeriodMins = 60
)

type StroomNodeMetric struct {
	Time     time.Time
	CpuUsage *resource.Quantity
}

func (in *StroomNodeMetric) IsZero() bool {
	return in.Time.IsZero() && in.CpuUsage == nil
}

type StroomNodeMetricMap struct {
	// Stored pod metrics
	Items map[string][]StroomNodeMetric

	// When a pod last had its tasks autoscaled
	LastScaled map[string]time.Time
}

func NewNodeMetricMap() StroomNodeMetricMap {
	return StroomNodeMetricMap{
		Items:      map[string][]StroomNodeMetric{},
		LastScaled: map[string]time.Time{},
	}
}

func (in *StroomNodeMetricMap) AddMetric(podName string, metric StroomNodeMetric) {
	if metrics, exists := in.Items[podName]; exists {
		in.Items[podName] = append(metrics, metric)
	} else {
		in.Items[podName] = []StroomNodeMetric{metric}
	}
}

func (in *StroomNodeMetricMap) IsScaleScheduled(podName string) bool {
	_, exists := in.LastScaled[podName]
	return exists
}

func (in *StroomNodeMetricMap) SetLastScaled(podName string, currentTime time.Time) {
	in.LastScaled[podName] = currentTime
}

func (in *StroomNodeMetricMap) DeletePodData(podName string) {
	delete(in.LastScaled, podName)
	delete(in.Items, podName)
}

func (in *StroomNodeMetricMap) ShouldScale(podName string, adjustmentIntervalMins int, currentTime time.Time) bool {
	if lastScaled, exists := in.LastScaled[podName]; exists {
		return currentTime.After(lastScaled.Add(time.Minute * time.Duration(adjustmentIntervalMins)))
	}

	return false
}

// AgeOff removes metrics in the map older than the specified retention period (in minutes)
func (in *StroomNodeMetricMap) AgeOff(retentionPeriodMins int, currentTime time.Time) {
	for podName, metricList := range in.Items {
		newMetricList := make([]StroomNodeMetric, 0)

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
func (in *StroomNodeMetricMap) GetSlidingWindowMean(podName string, slidingWindowIntervalMins int, currentTime time.Time, result *int64) bool {
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

		if valueCount > 0 {
			*result = sum / valueCount
			return true
		} else {
			return false
		}
	}

	return false
}
