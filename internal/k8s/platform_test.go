package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestUpdatePlatformDeploymentOnlyUpdatesPlatformContainer(t *testing.T) {
	replicas := int32(1)
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: platformDeploymentName, Namespace: platformDeploymentNamespace},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "sidecar", Image: "example.com/sidecar:1"},
			{Name: platformContainerName, Image: "registry.example.com/cylism-manager@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		}}}},
	})}

	previous, err := client.UpdatePlatformDeployment("registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 9)
	if err != nil {
		t.Fatal(err)
	}
	if previous != "registry.example.com/cylism-manager@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected previous image: %q", previous)
	}
	deployment, err := client.Clientset.AppsV1().Deployments(platformDeploymentNamespace).Get(t.Context(), platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Template.Spec.Containers[0].Image != "example.com/sidecar:1" || deployment.Spec.Template.Spec.Containers[1].Image != "registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || deployment.Spec.Template.Annotations[platformReleaseAnnotation] != "9" {
		t.Fatalf("unexpected deployment update: %#v", deployment.Spec.Template)
	}
}

func TestUpdatePlatformDeploymentRejectsMissingPlatformContainer(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: platformDeploymentName, Namespace: platformDeploymentNamespace}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "other", Image: "example.com/other"}}}}}})}
	if _, err := client.UpdatePlatformDeployment("registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 9); err == nil {
		t.Fatal("expected missing platform container rejection")
	}
}

func TestEnsurePlatformIngressUsesPlatformServiceAndManagedTLSSecret(t *testing.T) {
	client := &Client{Clientset: k8sfake.NewSimpleClientset()}
	if err := client.ensurePlatformIngress("console.example.com", "console-example-com-tls"); err != nil {
		t.Fatal(err)
	}
	ingress, err := client.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace).Get(t.Context(), platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ingress.Labels[platformEndpointLabel] != platformEndpointLabelValue || len(ingress.Spec.TLS) != 1 || ingress.Spec.TLS[0].SecretName != "console-example-com-tls" {
		t.Fatalf("unexpected platform ingress metadata: %#v", ingress)
	}
	path := ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0]
	if ingress.Spec.Rules[0].Host != "console.example.com" || path.Backend.Service.Name != platformDeploymentName || path.Backend.Service.Port.Number != 8080 {
		t.Fatalf("unexpected platform ingress route: %#v", ingress.Spec)
	}
}

func TestEnsurePlatformIngressRejectsConflictingHostname(t *testing.T) {
	pathType := networkingv1.PathTypePrefix
	conflicting := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "apps"},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			Host: "console.example.com",
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &pathType}},
			}},
		}}},
	}
	client := &Client{Clientset: k8sfake.NewSimpleClientset(conflicting)}
	if err := client.ensurePlatformIngress("console.example.com", "console-example-com-tls"); err == nil {
		t.Fatal("expected hostname conflict")
	}
}

func TestAdoptPlatformIngressPreservesControllerSettings(t *testing.T) {
	existing := platformIngress("console.example.com", "legacy-tls")
	existing.Labels = map[string]string{"manual": "true"}
	existing.Annotations = map[string]string{"traefik.ingress.kubernetes.io/router.middlewares": "default-auth@kubernetescrd"}
	className := "traefik"
	existing.Spec.IngressClassName = &className
	client := &Client{Clientset: k8sfake.NewSimpleClientset(existing)}
	if err := client.AdoptPlatformIngress("console.example.com", "console-example-com-tls"); err != nil {
		t.Fatal(err)
	}
	ingress, err := client.Clientset.NetworkingV1().Ingresses(platformDeploymentNamespace).Get(t.Context(), platformDeploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ingress.Labels[platformEndpointLabel] != platformEndpointLabelValue || ingress.Annotations["traefik.ingress.kubernetes.io/router.middlewares"] == "" || ingress.Spec.IngressClassName == nil || *ingress.Spec.IngressClassName != "traefik" || ingress.Spec.TLS[0].SecretName != "console-example-com-tls" {
		t.Fatalf("unexpected adopted ingress: %#v", ingress)
	}
}
