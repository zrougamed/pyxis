package k8s

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListJobs(t *testing.T) {
	completions := int32(1)
	cs := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "migrate",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
		Spec: batchv1.JobSpec{Completions: &completions},
		Status: batchv1.JobStatus{
			Succeeded: 1,
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			},
		},
	})
	c := NewClientWithInterface(cs)

	jobs, err := c.ListJobs(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Kind != "Job" || jobs[0].Name != "migrate" {
		t.Errorf("unexpected job: %+v", jobs[0])
	}
	if jobs[0].Status != "Complete" {
		t.Errorf("expected Complete, got %q", jobs[0].Status)
	}
	if jobs[0].Extra["completions"] != "1/1" {
		t.Errorf("expected completions 1/1, got %q", jobs[0].Extra["completions"])
	}
}

func TestListCronJobs(t *testing.T) {
	cs := fake.NewSimpleClientset(&batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nightly",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-24 * time.Hour)),
		},
		Spec: batchv1.CronJobSpec{Schedule: "0 2 * * *"},
	})
	c := NewClientWithInterface(cs)

	items, err := c.ListCronJobs(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListCronJobs: %v", err)
	}
	if len(items) != 1 || items[0].Name != "nightly" {
		t.Fatalf("unexpected cronjobs: %+v", items)
	}
	if items[0].Extra["schedule"] != "0 2 * * *" {
		t.Errorf("expected schedule, got %q", items[0].Extra["schedule"])
	}
	if items[0].Status != "Active" {
		t.Errorf("expected Active, got %q", items[0].Status)
	}
}

func TestListIngresses(t *testing.T) {
	class := "nginx"
	cs := fake.NewSimpleClientset(&networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "web",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &class,
			Rules: []networkingv1.IngressRule{
				{Host: "example.com"},
			},
		},
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{{IP: "1.2.3.4"}},
			},
		},
	})
	c := NewClientWithInterface(cs)

	items, err := c.ListIngresses(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListIngresses: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 ingress, got %d", len(items))
	}
	if items[0].Extra["hosts"] != "example.com" {
		t.Errorf("hosts: %q", items[0].Extra["hosts"])
	}
	if items[0].Status != "1.2.3.4" {
		t.Errorf("status: %q", items[0].Status)
	}
}

func TestListPersistentVolumeClaims(t *testing.T) {
	sc := "standard"
	cs := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "data",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-3 * time.Hour)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &sc,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	})
	c := NewClientWithInterface(cs)

	items, err := c.ListPersistentVolumeClaims(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListPersistentVolumeClaims: %v", err)
	}
	if len(items) != 1 || items[0].Status != "Bound" {
		t.Fatalf("unexpected pvcs: %+v", items)
	}
	if items[0].Extra["storageClass"] != "standard" {
		t.Errorf("storageClass: %q", items[0].Extra["storageClass"])
	}
}

func TestListPersistentVolumes(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pv-data",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")},
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              "standard",
			ClaimRef:                      &corev1.ObjectReference{Namespace: "default", Name: "data"},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	})
	c := NewClientWithInterface(cs)

	items, err := c.ListPersistentVolumes(context.Background())
	if err != nil {
		t.Fatalf("ListPersistentVolumes: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pv, got %d", len(items))
	}
	if items[0].Kind != "PersistentVolume" || items[0].Name != "pv-data" {
		t.Errorf("unexpected: %+v", items[0])
	}
	if items[0].Extra["capacity"] != "20Gi" {
		t.Errorf("capacity: %q", items[0].Extra["capacity"])
	}
	if items[0].Extra["claim"] != "default/data" {
		t.Errorf("claim: %q", items[0].Extra["claim"])
	}
}

func TestListNamespaceInfosAndCRUD(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "apps",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
			Labels:            map[string]string{"team": "platform"},
		},
		Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	})
	c := NewClientWithInterface(cs)

	items, err := c.ListNamespaceInfos(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaceInfos: %v", err)
	}
	if len(items) != 1 || items[0].Name != "apps" || items[0].Status != "Active" {
		t.Fatalf("unexpected: %+v", items)
	}

	if err := c.CreateNamespace(context.Background(), "new-ns"); err != nil {
		t.Fatalf("CreateNamespace: %v", err)
	}
	if err := c.DeleteNamespace(context.Background(), "new-ns"); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}
}

func TestLintYAMLClient(t *testing.T) {
	issues := LintYAMLClient("")
	if len(issues) == 0 {
		t.Fatal("expected empty manifest error")
	}
	issues = LintYAMLClient("not: [yaml")
	if len(issues) == 0 {
		t.Fatal("expected parse error")
	}
	issues = LintYAMLClient("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n")
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	issues = LintYAMLClient("kind: ConfigMap\nmetadata: {}\n")
	if len(issues) < 2 {
		t.Fatalf("expected apiVersion/name issues, got %+v", issues)
	}
}

func TestListHPAs(t *testing.T) {
	min := int32(2)
	cs := fake.NewSimpleClientset(&autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "api-hpa",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: &min,
			MaxReplicas: 10,
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: "api",
			},
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 3,
			DesiredReplicas: 4,
		},
	})
	c := NewClientWithInterface(cs)

	items, err := c.ListHPAs(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListHPAs: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 hpa, got %d", len(items))
	}
	if items[0].Status != "3/4" {
		t.Errorf("status: %q", items[0].Status)
	}
	if items[0].Extra["target"] != "Deployment/api" {
		t.Errorf("target: %q", items[0].Extra["target"])
	}
}

func TestListCRDs_NilConfig(t *testing.T) {
	c := NewClientWithInterface(fake.NewSimpleClientset())
	items, err := c.ListCRDs(context.Background())
	if err != nil {
		t.Fatalf("ListCRDs: %v", err)
	}
	if items != nil && len(items) != 0 {
		t.Errorf("expected empty list with nil config, got %+v", items)
	}
}

func TestListHelmReleases(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.nginx.v1",
				Namespace: "default",
				Labels: map[string]string{
					"owner":   "helm",
					"name":    "nginx",
					"version": "1",
					"status":  "superseded",
				},
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
			},
			Type: "helm.sh/release.v1",
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.nginx.v2",
				Namespace: "default",
				Labels: map[string]string{
					"owner":   "helm",
					"name":    "nginx",
					"version": "2",
					"status":  "deployed",
				},
				CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
			},
			Type: "helm.sh/release.v1",
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-secret",
				Namespace: "default",
				Labels:    map[string]string{"owner": "helm"},
			},
			Type: corev1.SecretTypeOpaque,
		},
	)
	c := NewClientWithInterface(cs)

	items, err := c.ListHelmReleases(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListHelmReleases: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 release (latest), got %d: %+v", len(items), items)
	}
	if items[0].Name != "nginx" || items[0].Status != "deployed" {
		t.Errorf("unexpected release: %+v", items[0])
	}
	if items[0].Extra["version"] != "2" {
		t.Errorf("expected version 2, got %q", items[0].Extra["version"])
	}
}

func TestParseHelmSecretName(t *testing.T) {
	name, ver, ok := parseHelmSecretName("sh.helm.release.v1.my-app.v3")
	if !ok || name != "my-app" || ver != 3 {
		t.Errorf("got name=%q ver=%d ok=%v", name, ver, ok)
	}
	_, _, ok = parseHelmSecretName("not-a-helm-secret")
	if ok {
		t.Error("expected parse failure")
	}
}

func TestDeleteDeployment(t *testing.T) {
	cs := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
	})
	c := NewClientWithInterface(cs)
	if err := c.DeleteDeployment(context.Background(), "default", "api"); err != nil {
		t.Fatalf("DeleteDeployment: %v", err)
	}
	_, err := cs.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected deployment to be deleted")
	}
}

func TestScaleStatefulSet(t *testing.T) {
	replicas := int32(1)
	cs := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
	})
	c := NewClientWithInterface(cs)
	if err := c.ScaleStatefulSet(context.Background(), "default", "db", 3); err != nil {
		t.Fatalf("ScaleStatefulSet: %v", err)
	}
	sts, err := cs.AppsV1().StatefulSets("default").Get(context.Background(), "db", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %v", sts.Spec.Replicas)
	}
}

func TestRestartStatefulSetAndDaemonSet(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
			Spec:       appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{}},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default"},
			Spec:       appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{}},
		},
	)
	c := NewClientWithInterface(cs)

	if err := c.RestartStatefulSet(context.Background(), "default", "db"); err != nil {
		t.Fatalf("RestartStatefulSet: %v", err)
	}
	sts, err := cs.AppsV1().StatefulSets("default").Get(context.Background(), "db", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get sts: %v", err)
	}
	if sts.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Error("expected restartedAt on statefulset")
	}

	if err := c.RestartDaemonSet(context.Background(), "default", "agent"); err != nil {
		t.Fatalf("RestartDaemonSet: %v", err)
	}
	ds, err := cs.AppsV1().DaemonSets("default").Get(context.Background(), "agent", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get ds: %v", err)
	}
	if ds.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Error("expected restartedAt on daemonset")
	}
}

func TestGetPodContainers(t *testing.T) {
	c := newTestClient(corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	})
	names, err := c.GetPodContainers(context.Background(), "default", "web")
	if err != nil {
		t.Fatalf("GetPodContainers: %v", err)
	}
	if len(names) != 2 || names[0] != "app" || names[1] != "sidecar" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestStartPortForward_NilConfig(t *testing.T) {
	c := NewClientWithInterface(fake.NewSimpleClientset())
	_, err := c.StartPortForward(context.Background(), "default", "pod", 8080, 80)
	if err == nil {
		t.Fatal("expected error with nil config")
	}
}

func TestExecInPod_NilConfig(t *testing.T) {
	c := NewClientWithInterface(fake.NewSimpleClientset())
	err := c.ExecInPod(context.Background(), "default", "pod", "main", []string{"ls"}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error with nil config")
	}
}

func TestMetricsHistory(t *testing.T) {
	h := NewMetricsHistory(3)
	h.Add(SparklineSample{CPUPercent: 10, MemPercent: 20})
	h.Add(SparklineSample{CPUPercent: 30, MemPercent: 40})
	h.Add(SparklineSample{CPUPercent: 50, MemPercent: 60})
	h.Add(SparklineSample{CPUPercent: 70, MemPercent: 80}) // overwrites first

	if h.Len() != 3 {
		t.Fatalf("expected len 3, got %d", h.Len())
	}
	cpu := h.CPUPercents()
	if len(cpu) != 3 || cpu[0] != 30 || cpu[2] != 70 {
		t.Errorf("unexpected cpu history: %v", cpu)
	}
}
