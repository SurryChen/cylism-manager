package logging

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
)

type FilterReader interface {
	ListRuntimePods(context.Context, string, string) ([]corev1.Pod, error)
	ListNodesContext(context.Context) ([]corev1.Node, error)
}

type PodFilter struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	NodeName   string   `json:"node_name,omitempty"`
	Containers []string `json:"containers"`
}

type FilterOptions struct {
	Namespaces []string
	Pods       []PodFilter
	Nodes      []string
}

func Filters(ctx context.Context, namespace string, reader FilterReader) (FilterOptions, error) {
	if reader == nil {
		return FilterOptions{}, nil
	}
	pods, err := reader.ListRuntimePods(ctx, namespace, "")
	if err != nil {
		return FilterOptions{}, err
	}
	nodes, err := reader.ListNodesContext(ctx)
	if err != nil {
		return FilterOptions{}, err
	}
	namespaceSet := map[string]struct{}{}
	options := make([]PodFilter, 0, len(pods))
	for _, pod := range pods {
		namespaceSet[pod.Namespace] = struct{}{}
		containers := make([]string, 0, len(pod.Spec.Containers))
		for _, container := range pod.Spec.Containers {
			containers = append(containers, container.Name)
		}
		sort.Strings(containers)
		options = append(options, PodFilter{Name: pod.Name, Namespace: pod.Namespace, NodeName: pod.Spec.NodeName, Containers: containers})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Namespace != options[j].Namespace {
			return options[i].Namespace < options[j].Namespace
		}
		return options[i].Name < options[j].Name
	})
	namespaces := make([]string, 0, len(namespaceSet))
	for name := range namespaceSet {
		namespaces = append(namespaces, name)
	}
	sort.Strings(namespaces)
	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	sort.Strings(nodeNames)
	return FilterOptions{Namespaces: namespaces, Pods: options, Nodes: nodeNames}, nil
}
