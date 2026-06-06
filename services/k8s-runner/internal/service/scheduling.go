package service

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func (r *kubernetesRunner) defaultPoolScheduling() (map[string]string, corev1.ResourceRequirements, error) {
	resources := corev1.ResourceRequirements{}
	if len(r.runtimePools) == 0 {
		return nil, resources, nil
	}
	pool, ok := r.runtimePools[defaultRuntimePoolName]
	if !ok {
		return nil, resources, nil
	}
	requests, err := resourceList(pool.Resources.Requests)
	if err != nil {
		return nil, resources, err
	}
	limits, err := resourceList(pool.Resources.Limits)
	if err != nil {
		return nil, resources, err
	}
	resources.Requests = requests
	resources.Limits = limits
	return cloneMap(pool.NodeSelector), resources, nil
}

func resourceList(values map[string]string) (corev1.ResourceList, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := corev1.ResourceList{}
	for name, value := range values {
		qty, err := resource.ParseQuantity(value)
		if err != nil {
			return nil, fmt.Errorf("parse resource %s=%s: %w", name, value, err)
		}
		out[corev1.ResourceName(name)] = qty
	}
	return out, nil
}
