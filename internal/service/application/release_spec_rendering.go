package application

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"path"
	"strings"
)

func RenderResources(context ApplicationContext, spec ReleaseSpec) (*RenderedResources, error) {
	if issues := ValidateReleaseSpec(spec); len(issues) > 0 {
		return nil, fmt.Errorf("发布定义无效: %s", issues[0].Message)
	}
	if context.ApplicationName == "" || context.Namespace == "" || context.ProjectName == "" || context.EnvironmentName == "" {
		return nil, fmt.Errorf("应用上下文不完整")
	}
	if err := ValidateApplicationName(context.ApplicationName); err != nil {
		return nil, err
	}
	workloadKind := context.WorkloadKind
	if workloadKind == "" {
		workloadKind = WorkloadKindDeployment
	}
	if workloadKind != WorkloadKindDeployment && workloadKind != WorkloadKindStatefulSet {
		return nil, fmt.Errorf("工作负载类型必须为 deployment 或 statefulset")
	}
	labels := managedLabels(context)
	result := &RenderedResources{SanitizedSpec: SanitizeReleaseSpec(spec)}
	configName := context.ApplicationName + "-config"
	secretName := context.ApplicationName + "-secret"

	config := enabledStringMap(spec.Config, spec.ConfigDisabled)
	secrets := enabledStringMap(spec.Secrets, spec.SecretsDisabled)
	if len(config) > 0 {
		result.ConfigMap = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: configName, Namespace: context.Namespace, Labels: labels}, Data: config}
	}
	if len(secrets) > 0 {
		if hasSecretValues(secrets) {
			result.Secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: context.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, StringData: secrets}
		}
	}
	if spec.RegistryID != 0 && spec.RegistryAuthType != "" && spec.RegistryAuthType != "anonymous" {
		pullSecret, err := imagePullSecret(context, labels, spec)
		if err != nil {
			return nil, err
		}
		result.ImagePullSecret = pullSecret
	}

	servicePorts := spec.Service.PortSpecs()
	container := corev1.Container{
		Name:    context.ApplicationName,
		Image:   spec.Image,
		Command: append([]string(nil), spec.Command...),
		Args:    append([]string(nil), spec.Args...),
		Ports:   renderContainerPorts(servicePorts),
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(spec.Resources.RequestsCPU), corev1.ResourceMemory: resource.MustParse(spec.Resources.RequestsMemory)},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(spec.Resources.LimitsCPU), corev1.ResourceMemory: resource.MustParse(spec.Resources.LimitsMemory)},
		},
	}
	if enabled, probeType := healthProbeEnabled(spec.Health.ReadinessEnabled, spec.Health.ReadinessType, spec.Health.ReadinessPath); enabled {
		container.ReadinessProbe = healthProbe(probeType, spec.Health.ReadinessPath, spec.ContainerPort)
	}
	if enabled, probeType := healthProbeEnabled(spec.Health.LivenessEnabled, spec.Health.LivenessType, spec.Health.LivenessPath); enabled {
		container.LivenessProbe = healthProbe(probeType, spec.Health.LivenessPath, spec.ContainerPort)
	}
	if result.ConfigMap != nil {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: configName}}})
	}
	if len(secrets) > 0 {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}}})
	}
	volumes := make([]corev1.Volume, 0, len(spec.Volumes)+len(spec.FileMounts))
	for index, volume := range spec.Volumes {
		name := fmt.Sprintf("pvc-%d", index)
		volumes = append(volumes, corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: volume.ClaimName, ReadOnly: volume.ReadOnly}}})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: name, MountPath: volume.MountPath, ReadOnly: volume.ReadOnly})
	}
	fileVolumes, fileVolumeMounts := renderFileMounts(resolveFileMountSources(spec.FileMounts, configName))
	volumes = append(volumes, fileVolumes...)
	container.VolumeMounts = append(container.VolumeMounts, fileVolumeMounts...)
	replicas := spec.Replicas
	podSpec := corev1.PodSpec{Containers: []corev1.Container{container}, Volumes: volumes}
	if spec.HostNetwork {
		podSpec.HostNetwork = true
		podSpec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}
	if spec.NodeName != "" {
		podSpec.NodeSelector = map[string]string{corev1.LabelHostname: spec.NodeName}
	}
	if result.ImagePullSecret != nil {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: result.ImagePullSecret.Name}}
	}
	podLabels := mergeLabels(labels, workloadSelector(context.ApplicationName))
	podLabels[ReleaseLabel] = fmt.Sprintf("%d", context.ReleaseSequence)
	podTemplate := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: podLabels}, Spec: podSpec}
	workloadMetadata := metav1.ObjectMeta{Name: context.ApplicationName, Namespace: context.Namespace, Labels: labels, Annotations: map[string]string{ReleaseLabel: fmt.Sprintf("%d", context.ReleaseSequence)}}
	if workloadKind == WorkloadKindStatefulSet {
		result.StatefulSet = &appsv1.StatefulSet{
			ObjectMeta: workloadMetadata,
			Spec: appsv1.StatefulSetSpec{
				ServiceName: context.ApplicationName,
				Replicas:    &replicas,
				Selector:    &metav1.LabelSelector{MatchLabels: workloadSelector(context.ApplicationName)},
				Template:    podTemplate,
			},
		}
	} else {
		result.Deployment = &appsv1.Deployment{
			ObjectMeta: workloadMetadata,
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: workloadSelector(context.ApplicationName)},
				Template: podTemplate,
			},
		}
		if spec.HostNetwork || len(spec.Volumes) > 0 {
			result.Deployment.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
		}
	}
	serviceType := corev1.ServiceType(normalizeServiceType(spec.Service.Type))
	serviceSpec := corev1.ServiceSpec{Type: serviceType, Selector: workloadSelector(context.ApplicationName), Ports: renderServicePorts(servicePorts)}
	if spec.Service.ExternalTrafficPolicy != "" {
		serviceSpec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyType(spec.Service.ExternalTrafficPolicy)
	}
	result.Service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: context.ApplicationName, Namespace: context.Namespace, Labels: labels},
		Spec:       serviceSpec,
	}

	if spec.Endpoint.Exposure == ExposurePublic {
		primaryTCPPort, _ := spec.Service.PrimaryTCPPort()
		result.Ingress = endpointIngress(context, spec.Endpoint, primaryTCPPort.Port)
	}
	return result, nil
}

func renderContainerPorts(servicePorts []ServicePortSpec) []corev1.ContainerPort {
	ports := make([]corev1.ContainerPort, 0, len(servicePorts))
	seen := make(map[string]struct{}, len(servicePorts))
	for _, servicePort := range servicePorts {
		protocol := normalizeServiceProtocol(servicePort.Protocol)
		key := fmt.Sprintf("%d/%s", servicePort.TargetPort, protocol)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ports = append(ports, corev1.ContainerPort{ContainerPort: servicePort.TargetPort, Protocol: corev1.Protocol(protocol)})
	}
	return ports
}

func renderServicePorts(servicePorts []ServicePortSpec) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(servicePorts))
	for _, servicePort := range servicePorts {
		port := corev1.ServicePort{Name: servicePort.Name, Protocol: corev1.Protocol(normalizeServiceProtocol(servicePort.Protocol)), Port: servicePort.Port, TargetPort: intstr.FromInt32(servicePort.TargetPort)}
		if servicePort.NodePort != 0 {
			port.NodePort = servicePort.NodePort
		}
		ports = append(ports, port)
	}
	return ports
}

type fileMountGroup struct {
	sourceType string
	sourceName string
	mountDir   string
	items      []corev1.KeyToPath
}

func renderFileMounts(fileMounts []FileMountSpec) ([]corev1.Volume, []corev1.VolumeMount) {
	groups := make([]fileMountGroup, 0, len(fileMounts))
	bySourceAndDirectory := make(map[string]int, len(fileMounts))
	for _, fileMount := range fileMounts {
		mountDir := path.Dir(fileMount.MountPath)
		key := fileMount.SourceType + "\x00" + fileMount.SourceName + "\x00" + mountDir
		index, exists := bySourceAndDirectory[key]
		if !exists {
			index = len(groups)
			bySourceAndDirectory[key] = index
			groups = append(groups, fileMountGroup{sourceType: fileMount.SourceType, sourceName: fileMount.SourceName, mountDir: mountDir})
		}
		groups[index].items = append(groups[index].items, corev1.KeyToPath{Key: fileMount.Key, Path: path.Base(fileMount.MountPath)})
	}
	volumes := make([]corev1.Volume, 0, len(groups))
	volumeMounts := make([]corev1.VolumeMount, 0, len(groups))
	for index, group := range groups {
		name := fmt.Sprintf("file-%d", index)
		volume := corev1.Volume{Name: name}
		if group.sourceType == FileMountSourceSecret || group.sourceType == FileMountSourceApplicationSecret {
			volume.Secret = &corev1.SecretVolumeSource{SecretName: group.sourceName, Items: group.items}
		} else {
			volume.ConfigMap = &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: group.sourceName}, Items: group.items}
		}
		volumes = append(volumes, volume)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: name, MountPath: group.mountDir, ReadOnly: true})
	}
	return volumes, volumeMounts
}

func resolveFileMountSources(fileMounts []FileMountSpec, applicationConfigName string) []FileMountSpec {
	resolved := make([]FileMountSpec, len(fileMounts))
	copy(resolved, fileMounts)
	for index := range resolved {
		if resolved[index].SourceType == FileMountSourceApplicationConfig {
			resolved[index].SourceType = FileMountSourceConfigMap
			resolved[index].SourceName = applicationConfigName
		}
		if resolved[index].SourceType == FileMountSourceApplicationSecret {
			resolved[index].SourceType = FileMountSourceSecret
			resolved[index].SourceName = strings.TrimSuffix(applicationConfigName, "-config") + "-secret"
		}
	}
	return resolved
}

func endpointIngress(context ApplicationContext, endpoint EndpointSpec, servicePort int32) *networkingv1.Ingress {
	return applicationEndpointsIngress(context, []model.ApplicationEndpoint{{
		Domain:        endpoint.Domain,
		Path:          endpoint.Path,
		TLSEnabled:    endpoint.TLSEnabled,
		TLSSecretName: endpoint.ManagedTLSSecretName,
	}}, servicePort)
}

func applicationEndpointsIngress(context ApplicationContext, endpoints []model.ApplicationEndpoint, servicePort int32) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: context.ApplicationName, Namespace: context.Namespace, Labels: managedLabels(context)},
		Spec:       networkingv1.IngressSpec{},
	}
	tlsEntries := make(map[string]int)
	for _, endpoint := range endpoints {
		if strings.TrimSpace(endpoint.Domain) == "" {
			continue
		}
		path := endpoint.Path
		if path == "" {
			path = "/"
		}
		backendPort := endpoint.ServicePort
		if backendPort <= 0 {
			backendPort = servicePort
		}
		ingress.Spec.Rules = append(ingress.Spec.Rules, networkingv1.IngressRule{
			Host: endpoint.Domain,
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
				Path: path, PathType: &pathType,
				Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: context.ApplicationName, Port: networkingv1.ServiceBackendPort{Number: backendPort}}},
			}}}},
		})
		if !endpoint.TLSEnabled {
			continue
		}
		tlsName := endpoint.TLSSecretName
		if tlsName == "" {
			tlsName = context.ApplicationName + "-tls"
		}
		if index, ok := tlsEntries[tlsName]; ok {
			ingress.Spec.TLS[index].Hosts = append(ingress.Spec.TLS[index].Hosts, endpoint.Domain)
			continue
		}
		tlsEntries[tlsName] = len(ingress.Spec.TLS)
		ingress.Spec.TLS = append(ingress.Spec.TLS, networkingv1.IngressTLS{Hosts: []string{endpoint.Domain}, SecretName: tlsName})
	}
	return ingress
}

func imagePullSecret(context ApplicationContext, labels map[string]string, spec ReleaseSpec) (*corev1.Secret, error) {
	if strings.TrimSpace(spec.RegistryEndpoint) == "" || strings.TrimSpace(spec.RegistryUsername) == "" || strings.TrimSpace(spec.RegistryCredential) == "" {
		return nil, fmt.Errorf("镜像仓库凭据不完整")
	}
	config, err := json.Marshal(map[string]map[string]map[string]string{"auths": {
		spec.RegistryEndpoint: {"username": spec.RegistryUsername, "password": spec.RegistryCredential, "auth": base64.StdEncoding.EncodeToString([]byte(spec.RegistryUsername + ":" + spec.RegistryCredential))},
	}})
	if err != nil {
		return nil, fmt.Errorf("生成镜像仓库凭据: %w", err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cylism-regcred-%d", spec.RegistryID), Namespace: context.Namespace, Labels: labels},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: config},
	}, nil
}

func managedLabels(context ApplicationContext) map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue, ProjectLabel: fmt.Sprintf("project-%d", context.ProjectID), ApplicationNameLabel: context.ApplicationName,
		EnvironmentLabel: fmt.Sprintf("environment-%d", context.EnvironmentID),
	}
}

// ValidateApplicationName ensures the application can be used as a Kubernetes resource name and label value.

func workloadSelector(applicationName string) map[string]string {
	return map[string]string{ApplicationNameLabel: applicationName}
}

func httpProbe(path string, port int32) *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromInt32(port)}}, InitialDelaySeconds: 5, PeriodSeconds: 10}
}

func healthProbeEnabled(enabled bool, probeType, path string) (bool, string) {
	if probeType == "" {
		probeType = "http"
	}
	return enabled, probeType
}

func healthProbe(probeType, path string, port int32) *corev1.Probe {
	if probeType == "tcp" {
		return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(port)}}, InitialDelaySeconds: 5, PeriodSeconds: 10}
	}
	return httpProbe(path, port)
}

func certificateResource(context ApplicationContext, domain, secretName, issuerRef, issuerKind string, labels map[string]string) *unstructured.Unstructured {
	if issuerKind == "" {
		issuerKind = "ClusterIssuer"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{"name": context.ApplicationName + "-tls", "namespace": context.Namespace, "labels": labels},
		"spec":     map[string]interface{}{"secretName": secretName, "dnsNames": []interface{}{domain}, "issuerRef": map[string]interface{}{"name": issuerRef, "kind": issuerKind}},
	}}
}
