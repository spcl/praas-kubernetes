/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
)

// MakeMetric constructs a Metric resource from a PodAutoscaler
func MakeMetric(pa *autoscalingv1alpha1.PodAutoscaler, metricSvc string) *autoscalingv1alpha1.Metric {
	return &autoscalingv1alpha1.Metric{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       pa.Namespace,
			Name:            pa.Name,
			Annotations:     kmeta.CopyMap(pa.Annotations),
			Labels:          kmeta.CopyMap(pa.Labels),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(pa)},
		},
		Spec: autoscalingv1alpha1.MetricSpec{
			ScrapeTarget: metricSvc,
			StableWindow: time.Second,
			PanicWindow:  time.Second,
		},
	}
}
