package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestClient(pods ...corev1.Pod) *Client {
	cs := fake.NewSimpleClientset()
	for i := range pods {
		_, _ = cs.CoreV1().Pods(pods[i].Namespace).Create(
			context.Background(), &pods[i], metav1.CreateOptions{},
		)
	}
	return NewClientWithInterface(cs)
}

func makePod(name, namespace string, phase corev1.PodPhase, image string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
			Containers: []corev1.Container{
				{Name: "main", Image: image},
			},
		},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Image:        image,
					Ready:        phase == corev1.PodRunning,
					RestartCount: 0,
					State: corev1.ContainerState{
						Running: func() *corev1.ContainerStateRunning {
							if phase == corev1.PodRunning {
								return &corev1.ContainerStateRunning{
									StartedAt: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
								}
							}
							return nil
						}(),
					},
				},
			},
		},
	}
}

func TestListPods_All(t *testing.T) {
	c := newTestClient(
		makePod("nginx", "default", corev1.PodRunning, "nginx:latest"),
		makePod("redis", "default", corev1.PodPending, "redis:7"),
		makePod("api", "production", corev1.PodFailed, "myapi:v1"),
	)

	pods, err := c.ListPods(context.Background(), "", PodFilterAll)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 3 {
		t.Errorf("expected 3 pods, got %d", len(pods))
	}
}

func TestListPods_Running(t *testing.T) {
	c := newTestClient(
		makePod("nginx", "default", corev1.PodRunning, "nginx:latest"),
		makePod("redis", "default", corev1.PodPending, "redis:7"),
	)

	pods, err := c.ListPods(context.Background(), "", PodFilterRunning)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 1 {
		t.Errorf("expected 1 running pod, got %d", len(pods))
	}
	if pods[0].Name != "nginx" {
		t.Errorf("expected nginx pod, got %s", pods[0].Name)
	}
}

func TestListPods_NotRunning(t *testing.T) {
	c := newTestClient(
		makePod("nginx", "default", corev1.PodRunning, "nginx:latest"),
		makePod("redis", "default", corev1.PodPending, "redis:7"),
		makePod("api", "default", corev1.PodFailed, "myapi:v1"),
	)

	pods, err := c.ListPods(context.Background(), "", PodFilterNotRunning)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 2 {
		t.Errorf("expected 2 non-running pods, got %d", len(pods))
	}
}

func TestListPods_ByNamespace(t *testing.T) {
	c := newTestClient(
		makePod("nginx", "default", corev1.PodRunning, "nginx:latest"),
		makePod("redis", "production", corev1.PodRunning, "redis:7"),
	)

	pods, err := c.ListPods(context.Background(), "default", PodFilterAll)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 1 {
		t.Errorf("expected 1 pod in default namespace, got %d", len(pods))
	}
}

func TestGetPodEnvVars_InlineEnv(t *testing.T) {
	cs := fake.NewSimpleClientset()

	// Create the secret so it can be resolved.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-secret", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("s3cret123")},
	}
	_, _ = cs.CoreV1().Secrets("default").Create(context.Background(), secret, metav1.CreateOptions{})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
					Env: []corev1.EnvVar{
						{Name: "DB_HOST", Value: "localhost"},
						{Name: "DB_PORT", Value: "5432"},
						{
							Name: "DB_PASSWORD",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "db-secret"},
									Key:                  "password",
								},
							},
						},
					},
				},
			},
		},
	}
	_, err := cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("creating test pod: %v", err)
	}

	c := NewClientWithInterface(cs)
	containers, err := c.GetPodEnvVars(context.Background(), "default", "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	envs := containers[0].EnvVars
	if len(envs) != 3 {
		t.Fatalf("expected 3 env vars, got %d", len(envs))
	}

	if envs[0].Name != "DB_HOST" || envs[0].Value != "localhost" {
		t.Errorf("unexpected env var: %+v", envs[0])
	}
	// Secret refs are now resolved to actual values.
	if envs[2].Value != "s3cret123" {
		t.Errorf("expected resolved secret value 's3cret123', got %q", envs[2].Value)
	}
	if envs[2].ValueFrom != "secret:db-secret/password" {
		t.Errorf("unexpected value source: %s", envs[2].ValueFrom)
	}
}

func TestGetPodEnvVars_EnvFromConfigMap(t *testing.T) {
	cs := fake.NewSimpleClientset()

	// Create the ConfigMap that envFrom references.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "default"},
		Data:       map[string]string{"LOG_LEVEL": "debug", "APP_PORT": "8080"},
	}
	_, _ = cs.CoreV1().ConfigMaps("default").Create(context.Background(), cm, metav1.CreateOptions{})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
					EnvFrom: []corev1.EnvFromSource{
						{ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
						}},
					},
				},
			},
		},
	}
	_, _ = cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	containers, err := c.GetPodEnvVars(context.Background(), "default", "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envs := containers[0].EnvVars
	if len(envs) != 2 {
		t.Fatalf("expected 2 env vars from configmap, got %d: %+v", len(envs), envs)
	}

	// Check that we got expanded keys from the configmap.
	found := map[string]bool{}
	for _, ev := range envs {
		found[ev.Name] = true
	}
	if !found["LOG_LEVEL"] || !found["APP_PORT"] {
		t.Errorf("expected LOG_LEVEL and APP_PORT, got %+v", envs)
	}
}

func TestGetPodEnvVars_EnvFromSecret(t *testing.T) {
	cs := fake.NewSimpleClientset()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "default"},
		Data:       map[string][]byte{"DB_USER": []byte("admin"), "DB_PASS": []byte("s3cret")},
	}
	_, _ = cs.CoreV1().Secrets("default").Create(context.Background(), secret, metav1.CreateOptions{})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
					EnvFrom: []corev1.EnvFromSource{
						{SecretRef: &corev1.SecretEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "db-creds"},
						}},
					},
				},
			},
		},
	}
	_, _ = cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	containers, err := c.GetPodEnvVars(context.Background(), "default", "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envs := containers[0].EnvVars
	if len(envs) != 2 {
		t.Fatalf("expected 2 env vars from secret, got %d", len(envs))
	}

	// Secret values are now resolved to actual content.
	found := map[string]string{}
	for _, ev := range envs {
		found[ev.Name] = ev.Value
	}
	if found["DB_USER"] != "admin" {
		t.Errorf("expected DB_USER='admin', got %q", found["DB_USER"])
	}
	if found["DB_PASS"] != "s3cret" {
		t.Errorf("expected DB_PASS='s3cret', got %q", found["DB_PASS"])
	}
}

func TestGetPodEnvVars_EnvFromWithPrefix(t *testing.T) {
	cs := fake.NewSimpleClientset()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Data:       map[string]string{"HOST": "localhost"},
	}
	_, _ = cs.CoreV1().ConfigMaps("default").Create(context.Background(), cm, metav1.CreateOptions{})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
					EnvFrom: []corev1.EnvFromSource{
						{
							Prefix: "MYAPP_",
							ConfigMapRef: &corev1.ConfigMapEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
							},
						},
					},
				},
			},
		},
	}
	_, _ = cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	containers, err := c.GetPodEnvVars(context.Background(), "default", "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if containers[0].EnvVars[0].Name != "MYAPP_HOST" {
		t.Errorf("expected prefixed key 'MYAPP_HOST', got %q", containers[0].EnvVars[0].Name)
	}
}

func TestGetPodEnvVars_InitContainers(t *testing.T) {
	cs := fake.NewSimpleClientset()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "init-db",
					Image: "busybox",
					Env:   []corev1.EnvVar{{Name: "INIT_MODE", Value: "migrate"}},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
					Env:   []corev1.EnvVar{{Name: "APP_MODE", Value: "serve"}},
				},
			},
		},
	}
	_, _ = cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	containers, err := c.GetPodEnvVars(context.Background(), "default", "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(containers) != 2 {
		t.Fatalf("expected 2 containers (init + regular), got %d", len(containers))
	}
	if containers[0].Name != "init-db (init)" {
		t.Errorf("expected init container marked with (init), got %q", containers[0].Name)
	}
	if containers[0].EnvVars[0].Name != "INIT_MODE" {
		t.Errorf("expected INIT_MODE, got %q", containers[0].EnvVars[0].Name)
	}
}

func TestGetPodEnvVars_MissingConfigMap(t *testing.T) {
	cs := fake.NewSimpleClientset()

	// Pod references a configmap that doesn't exist.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
					EnvFrom: []corev1.EnvFromSource{
						{ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "nonexistent"},
						}},
					},
				},
			},
		},
	}
	_, _ = cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	containers, err := c.GetPodEnvVars(context.Background(), "default", "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still return a result with an "unreadable" placeholder.
	envs := containers[0].EnvVars
	if len(envs) != 1 {
		t.Fatalf("expected 1 fallback entry, got %d", len(envs))
	}
	if envs[0].Name != "*" {
		t.Errorf("expected wildcard fallback name '*', got %q", envs[0].Name)
	}
}

func TestGetPodEnvVars_ConfigMapKeyRef_Resolved(t *testing.T) {
	cs := fake.NewSimpleClientset()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"},
		Data:       map[string]string{"db_host": "pg.example.com"},
	}
	_, _ = cs.CoreV1().ConfigMaps("default").Create(context.Background(), cm, metav1.CreateOptions{})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
					Env: []corev1.EnvVar{
						{
							Name: "DB_HOST",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"},
									Key:                  "db_host",
								},
							},
						},
					},
				},
			},
		},
	}
	_, _ = cs.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	c := NewClientWithInterface(cs)
	containers, err := c.GetPodEnvVars(context.Background(), "default", "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev := containers[0].EnvVars[0]
	if ev.Value != "pg.example.com" {
		t.Errorf("expected resolved value 'pg.example.com', got %q", ev.Value)
	}
	if ev.ValueFrom != "configmap:my-cm/db_host" {
		t.Errorf("expected source description, got %q", ev.ValueFrom)
	}
}

func TestToPodInfo(t *testing.T) {
	pod := makePod("test", "default", corev1.PodRunning, "nginx:1.25")
	info := toPodInfo(pod)

	if info.Name != "test" {
		t.Errorf("expected name 'test', got %q", info.Name)
	}
	if info.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", info.Namespace)
	}
	if info.Phase != corev1.PodRunning {
		t.Errorf("expected Running phase, got %v", info.Phase)
	}
	if len(info.Images) != 1 || info.Images[0] != "nginx:1.25" {
		t.Errorf("unexpected images: %v", info.Images)
	}
	if info.Ready != "1/1" {
		t.Errorf("expected ready 1/1, got %s", info.Ready)
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		phase  corev1.PodPhase
		filter PodPhaseFilter
		want   bool
	}{
		{corev1.PodRunning, PodFilterAll, true},
		{corev1.PodRunning, PodFilterRunning, true},
		{corev1.PodPending, PodFilterRunning, false},
		{corev1.PodPending, PodFilterNotRunning, true},
		{corev1.PodFailed, PodFilterFailed, true},
		{corev1.PodRunning, PodFilterFailed, false},
		{corev1.PodPending, PodFilterPending, true},
		{corev1.PodSucceeded, PodFilterSucceeded, true},
	}

	for _, tt := range tests {
		got := matchesFilter(tt.phase, tt.filter)
		if got != tt.want {
			t.Errorf("matchesFilter(%v, %v) = %v, want %v", tt.phase, tt.filter, got, tt.want)
		}
	}
}

func TestCurrentContext(t *testing.T) {
	c := NewClientWithInterface(fake.NewSimpleClientset())
	if ctx := c.CurrentContext(); ctx != "test-context" {
		t.Errorf("expected 'test-context', got %q", ctx)
	}
}

func TestDescribeValueSource(t *testing.T) {
	tests := []struct {
		name string
		src  *corev1.EnvVarSource
		want string
	}{
		{
			name: "configmap",
			src: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"},
					Key:                  "key1",
				},
			},
			want: "configmap:my-cm/key1",
		},
		{
			name: "secret",
			src: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "my-sec"},
					Key:                  "pass",
				},
			},
			want: "secret:my-sec/pass",
		},
		{
			name: "field",
			src: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
			want: "field:metadata.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeValueSource(tt.src)
			if got != tt.want {
				t.Errorf("describeValueSource() = %q, want %q", got, tt.want)
			}
		})
	}
}
