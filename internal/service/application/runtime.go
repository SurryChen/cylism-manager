package application

import (
	"context"
	"sort"
	"strconv"

	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// RuntimeReader is the minimal Kubernetes read capability required by
// application runtime views. It deliberately exposes no client, mutation, or
// reconciler details.
type RuntimeReader interface {
	ListRuntimeServices(context.Context, string) ([]corev1.Service, error)
	ListRuntimeDeployments(context.Context, string) ([]appsv1.Deployment, error)
	ListRuntimeStatefulSets(context.Context, string) ([]appsv1.StatefulSet, error)
	ListRuntimePods(context.Context, string, string) ([]corev1.Pod, error)
}

type RuntimeInfo struct {
	Status          string            `json:"status"`
	ServiceName     string            `json:"service_name"`
	Service         *ServiceRuntime   `json:"service,omitempty"`
	LatestRelease   *ReleaseSummary   `json:"latest_release,omitempty"`
	DesiredReplicas int32             `json:"desired_replicas"`
	ReadyReplicas   int32             `json:"ready_replicas"`
	AvailablePods   int32             `json:"available_pods"`
	ReadyPods       []ReadyPodRuntime `json:"ready_pods"`
	ReadyNodes      []string          `json:"ready_nodes"`
}

type ServiceRuntime struct {
	Name                  string               `json:"name"`
	Type                  string               `json:"type"`
	Ports                 []ServicePortRuntime `json:"ports"`
	LoadBalancerAddresses []string             `json:"load_balancer_addresses"`
}

type ServicePortRuntime struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"node_port,omitempty"`
}

type ReleaseSummary struct {
	ID       uint   `json:"id"`
	Sequence uint   `json:"sequence"`
	Version  string `json:"version,omitempty"`
	Status   string `json:"status"`
}

type ReadyPodRuntime struct {
	Name     string `json:"name"`
	NodeName string `json:"node_name"`
}

type WorkspaceRuntimeSummary struct {
	Status    string `json:"status"`
	ReadyPods int32  `json:"ready_pods"`
	TotalPods int32  `json:"total_pods"`
}

// ApplicationRuntimeInfos batches Kubernetes reads per namespace. Missing
// cluster access remains a view concern represented by unavailable statuses.
func (s *QueryService) ApplicationRuntimeInfos(ctx context.Context, reader RuntimeReader, applications []model.Application, releases map[uint][]model.Release) map[uint]RuntimeInfo {
	result := make(map[uint]RuntimeInfo, len(applications))
	byNamespace := make(map[string][]model.Application)
	for _, app := range applications {
		runtime := RuntimeInfo{ServiceName: app.Name, ReadyPods: []ReadyPodRuntime{}, ReadyNodes: []string{}}
		if latest := latestRelease(releases[app.ID]); latest != nil {
			runtime.LatestRelease = latest
			runtime.Status = "unavailable"
		} else {
			runtime.Status = "not_released"
		}
		result[app.ID] = runtime
		byNamespace[app.Environment.Namespace] = append(byNamespace[app.Environment.Namespace], app)
	}
	if reader == nil {
		return result
	}
	for namespace, namespaceApplications := range byNamespace {
		collectNamespaceRuntime(ctx, reader, namespace, namespaceApplications, result)
	}
	return result
}

func (s *QueryService) WorkspacePodStates(ctx context.Context, reader RuntimeReader, namespace string) (map[string]WorkspaceRuntimeSummary, bool) {
	states := make(map[string]WorkspaceRuntimeSummary)
	if reader == nil {
		return states, false
	}
	pods, err := reader.ListRuntimePods(ctx, namespace, ManagedByLabel+"="+ManagedByValue)
	if err != nil {
		return states, false
	}
	for _, pod := range pods {
		applicationName, release := pod.Labels[ApplicationNameLabel], pod.Labels[ReleaseLabel]
		sequence, err := strconv.ParseUint(release, 10, 64)
		if applicationName == "" || err != nil {
			continue
		}
		key := WorkspacePodKey(applicationName, uint(sequence))
		state := states[key]
		state.TotalPods++
		if podReady(pod) {
			state.ReadyPods++
		}
		states[key] = state
	}
	for key, state := range states {
		if state.TotalPods > 0 && state.ReadyPods == state.TotalPods {
			state.Status = "running"
		} else {
			state.Status = "degraded"
		}
		states[key] = state
	}
	return states, true
}

func WorkspacePodKey(applicationName string, sequence uint) string {
	return applicationName + "\x00" + strconv.FormatUint(uint64(sequence), 10)
}

func collectNamespaceRuntime(ctx context.Context, reader RuntimeReader, namespace string, applications []model.Application, result map[uint]RuntimeInfo) {
	services, serviceErr := reader.ListRuntimeServices(ctx, namespace)
	deployments, deploymentErr := reader.ListRuntimeDeployments(ctx, namespace)
	statefulSets, statefulSetErr := reader.ListRuntimeStatefulSets(ctx, namespace)
	pods, podErr := reader.ListRuntimePods(ctx, namespace, "")
	serviceByName := make(map[string]corev1.Service, len(services))
	if serviceErr == nil {
		for _, service := range services {
			serviceByName[service.Name] = service
		}
	}
	deploymentByName := make(map[string]appsv1.Deployment, len(deployments))
	if deploymentErr == nil {
		for _, deployment := range deployments {
			deploymentByName[deployment.Name] = deployment
		}
	}
	statefulSetByName := make(map[string]appsv1.StatefulSet, len(statefulSets))
	if statefulSetErr == nil {
		for _, statefulSet := range statefulSets {
			statefulSetByName[statefulSet.Name] = statefulSet
		}
	}
	podsByApplication := make(map[string][]corev1.Pod)
	if podErr == nil {
		for _, pod := range pods {
			if name := pod.Labels[ApplicationNameLabel]; name != "" {
				podsByApplication[name] = append(podsByApplication[name], pod)
			}
		}
	}
	for _, app := range applications {
		runtime := result[app.ID]
		if service, ok := serviceByName[app.Name]; ok {
			runtime.Service = serviceRuntime(service)
		}
		if app.WorkloadKind == WorkloadKindStatefulSet {
			if workload, ok := statefulSetByName[app.Name]; ok {
				runtime.DesiredReplicas = replicasValue(workload.Spec.Replicas)
				runtime.ReadyReplicas = workload.Status.ReadyReplicas
				runtime.AvailablePods = workload.Status.AvailableReplicas
			}
		} else if workload, ok := deploymentByName[app.Name]; ok {
			runtime.DesiredReplicas = replicasValue(workload.Spec.Replicas)
			runtime.ReadyReplicas = workload.Status.ReadyReplicas
			runtime.AvailablePods = workload.Status.AvailableReplicas
		}
		if podErr == nil {
			nodes := make(map[string]struct{})
			for _, pod := range podsByApplication[app.Name] {
				if !podReady(pod) {
					continue
				}
				runtime.ReadyPods = append(runtime.ReadyPods, ReadyPodRuntime{Name: pod.Name, NodeName: pod.Spec.NodeName})
				if pod.Spec.NodeName != "" {
					nodes[pod.Spec.NodeName] = struct{}{}
				}
			}
			for node := range nodes {
				runtime.ReadyNodes = append(runtime.ReadyNodes, node)
			}
			sort.Slice(runtime.ReadyPods, func(i, j int) bool { return runtime.ReadyPods[i].Name < runtime.ReadyPods[j].Name })
			sort.Strings(runtime.ReadyNodes)
		}
		if serviceErr != nil || deploymentErr != nil || statefulSetErr != nil || podErr != nil {
			runtime.Status = "unavailable"
		} else if runtime.LatestRelease == nil {
			runtime.Status = "not_released"
		} else if runtime.DesiredReplicas > 0 && runtime.ReadyReplicas >= runtime.DesiredReplicas {
			runtime.Status = "running"
		} else {
			runtime.Status = "degraded"
		}
		result[app.ID] = runtime
	}
}

func latestRelease(releases []model.Release) *ReleaseSummary {
	if len(releases) == 0 {
		return nil
	}
	latest := releases[0]
	for _, release := range releases[1:] {
		if release.Sequence > latest.Sequence {
			latest = release
		}
	}
	return &ReleaseSummary{ID: latest.ID, Sequence: latest.Sequence, Version: latest.Version, Status: latest.Status}
}

func serviceRuntime(service corev1.Service) *ServiceRuntime {
	ports := make([]ServicePortRuntime, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		targetPort := port.TargetPort.String()
		if targetPort == "0" {
			targetPort = strconv.Itoa(int(port.Port))
		}
		ports = append(ports, ServicePortRuntime{Name: port.Name, Port: port.Port, TargetPort: targetPort, Protocol: string(port.Protocol), NodePort: port.NodePort})
	}
	addresses := make([]string, 0, len(service.Status.LoadBalancer.Ingress))
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			addresses = append(addresses, ingress.IP)
		} else if ingress.Hostname != "" {
			addresses = append(addresses, ingress.Hostname)
		}
	}
	sort.Strings(addresses)
	return &ServiceRuntime{Name: service.Name, Type: string(service.Spec.Type), Ports: ports, LoadBalancerAddresses: addresses}
}

func replicasValue(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

func podReady(pod corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning || len(pod.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
	}
	return true
}
