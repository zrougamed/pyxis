package k8s

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }

func TestGetConfigMapData(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "default"},
		Data: map[string]string{
			"config.yaml": "server:\n  port: 8080\n  host: 0.0.0.0",
			"log_level":   "debug",
		},
		BinaryData: map[string][]byte{
			"cert.pem": []byte("binary-stuff"),
		},
	}
	_, _ = cs.CoreV1().ConfigMaps("default").Create(context.Background(), cm, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	manifest, err := c.GetConfigMapData(context.Background(), "default", "app-config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest.Kind != "ConfigMap" {
		t.Errorf("expected kind ConfigMap, got %q", manifest.Kind)
	}
	if !strings.Contains(manifest.Content, "config.yaml") {
		t.Error("expected config.yaml key in output")
	}
	if !strings.Contains(manifest.Content, "port: 8080") {
		t.Error("expected config value in output")
	}
	if !strings.Contains(manifest.Content, "log_level") {
		t.Error("expected log_level key in output")
	}
	if !strings.Contains(manifest.Content, "cert.pem (binary") {
		t.Error("expected binary key indicator in output")
	}
}

func TestGetSecretData(t *testing.T) {
	cs := fake.NewSimpleClientset()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("s3cret"),
		},
	}
	_, _ = cs.CoreV1().Secrets("default").Create(context.Background(), secret, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	manifest, err := c.GetSecretData(context.Background(), "default", "db-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest.Kind != "Secret" {
		t.Errorf("expected kind Secret, got %q", manifest.Kind)
	}
	if !strings.Contains(manifest.Content, "admin") {
		t.Error("expected decoded username in output")
	}
	if !strings.Contains(manifest.Content, "s3cret") {
		t.Error("expected decoded password in output")
	}
	if !strings.Contains(manifest.Content, "Opaque") {
		t.Error("expected secret type in output")
	}
}

func TestGetDeploymentCompact(t *testing.T) {
	cs := fake.NewSimpleClientset()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web",
			Namespace: "production",
			Labels:    map[string]string{"app": "web", "team": "platform"},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "huge-json-blob",
				"custom-annotation": "keep-this",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
					MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
				Spec: corev1.PodSpec{
					ServiceAccountName: "web-sa",
					Containers: []corev1.Container{
						{
							Name:  "web",
							Image: "nginx:1.25",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 80, Protocol: corev1.ProtocolTCP, Name: "http"},
							},
							Env: []corev1.EnvVar{
								{Name: "LOG_LEVEL", Value: "info"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromInt(80),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "config", MountPath: "/etc/config", ReadOnly: true},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "web-config"},
								},
							},
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     3,
			UpdatedReplicas:   3,
			AvailableReplicas: 3,
		},
	}
	_, _ = cs.AppsV1().Deployments("production").Create(context.Background(), dep, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	manifest, err := c.GetDeploymentCompact(context.Background(), "production", "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := manifest.Content

	// Should contain essential fields.
	checks := []string{
		"Deployment: production/web",
		"replicas: 3",
		"strategy: RollingUpdate",
		"maxSurge: 25%",
		"image: nginx:1.25",
		"80/TCP (http)",
		"LOG_LEVEL: info",
		"livenessProbe:",
		"/healthz",
		"config → /etc/config (ro)",
		"configMap: web-config",
		"serviceAccountName: web-sa",
		"custom-annotation: keep-this",
		"3/3 ready",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("compact output missing %q", check)
		}
	}

	// Should NOT contain auto-generated noise.
	noise := []string{
		"last-applied-configuration",
		"huge-json-blob",
	}
	for _, n := range noise {
		if strings.Contains(content, n) {
			t.Errorf("compact output should not contain %q", n)
		}
	}
}

func TestGetPodCompact(t *testing.T) {
	cs := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc123",
			Namespace: "default",
			Labels:    map[string]string{"app": "web"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "default",
			NodeName:           "node-1",
			RestartPolicy:      corev1.RestartPolicyAlways,
			Containers: []corev1.Container{
				{
					Name:    "web",
					Image:   "nginx:1.25",
					Command: []string{"nginx", "-g", "daemon off;"},
					Ports:   []corev1.ContainerPort{{ContainerPort: 80}},
					SecurityContext: &corev1.SecurityContext{
						RunAsNonRoot:           boolPtr(true),
						ReadOnlyRootFilesystem: boolPtr(true),
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.1.5",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "web",
					Ready:        true,
					RestartCount: 2,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	_, _ = cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	manifest, err := c.GetPodCompact(context.Background(), "default", "web-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := manifest.Content

	checks := []string{
		"Pod: default/web-abc123",
		"Phase: Running",
		"Node: node-1",
		"IP: 10.0.1.5",
		"image: nginx:1.25",
		"daemon off;",
		"runAsNonRoot",
		"readOnlyRootFs",
		"restarts=2",
		"serviceAccountName: default",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("pod compact output missing %q", check)
		}
	}
}

func TestGetStatefulSetCompact(t *testing.T) {
	cs := fake.NewSimpleClientset()
	sc := "standard"
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    int32Ptr(3),
			ServiceName: "redis-headless",
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "redis"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "redis"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "redis", Image: "redis:7"},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: &sc,
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("10Gi"),
							},
						},
					},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 3},
	}
	_, _ = cs.AppsV1().StatefulSets("default").Create(context.Background(), sts, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	manifest, err := c.GetStatefulSetCompact(context.Background(), "default", "redis")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"StatefulSet: default/redis",
		"serviceName: redis-headless",
		"replicas: 3",
		"image: redis:7",
		"volumeClaimTemplates:",
		"name: data",
		"ReadWriteOnce",
		"10Gi",
		"storageClassName: standard",
	}
	for _, check := range checks {
		if !strings.Contains(manifest.Content, check) {
			t.Errorf("statefulset compact output missing %q", check)
		}
	}
}

func TestGetDaemonSetCompact(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "kube-system"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "fluentd"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "fluentd"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "fluentd", Image: "fluentd:v1.16"},
					},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			NumberReady:            5,
		},
	}
	_, _ = cs.AppsV1().DaemonSets("kube-system").Create(context.Background(), ds, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	manifest, err := c.GetDaemonSetCompact(context.Background(), "kube-system", "fluentd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"DaemonSet: kube-system/fluentd",
		"5 desired, 5 ready",
		"image: fluentd:v1.16",
	}
	for _, check := range checks {
		if !strings.Contains(manifest.Content, check) {
			t.Errorf("daemonset compact output missing %q", check)
		}
	}
}

func TestCleanAnnotations(t *testing.T) {
	annotations := map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "big-json",
		"deployment.kubernetes.io/revision":                "5",
		"my-custom-annotation":                             "keep",
		"another-annotation":                               "also-keep",
	}

	cleaned := cleanAnnotations(annotations)
	if len(cleaned) != 2 {
		t.Errorf("expected 2 cleaned annotations, got %d", len(cleaned))
	}
	if cleaned["my-custom-annotation"] != "keep" {
		t.Error("expected my-custom-annotation to be kept")
	}
	if _, ok := cleaned["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Error("expected last-applied-configuration to be removed")
	}
}

func TestCleanAnnotations_Empty(t *testing.T) {
	if result := cleanAnnotations(nil); result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
	if result := cleanAnnotations(map[string]string{}); result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestPtrInt32(t *testing.T) {
	if v := ptrInt32(nil); v != 1 {
		t.Errorf("expected 1 for nil, got %d", v)
	}
	p := int32(5)
	if v := ptrInt32(&p); v != 5 {
		t.Errorf("expected 5, got %d", v)
	}
}

func TestListStatefulSets(t *testing.T) {
	cs := fake.NewSimpleClientset()
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "redis", Image: "redis:7"}},
				},
			},
		},
	}
	_, _ = cs.AppsV1().StatefulSets("default").Create(context.Background(), sts, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	resources, err := c.ListStatefulSets(context.Background(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 statefulset, got %d", len(resources))
	}
	if resources[0].Name != "redis" {
		t.Errorf("expected 'redis', got %q", resources[0].Name)
	}
}

func TestListDaemonSets(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "kube-system"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "fluentd", Image: "fluentd:v1"}},
				},
			},
		},
	}
	_, _ = cs.AppsV1().DaemonSets("kube-system").Create(context.Background(), ds, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	resources, err := c.ListDaemonSets(context.Background(), "kube-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 daemonset, got %d", len(resources))
	}
}

func boolPtr(b bool) *bool { return &b }
