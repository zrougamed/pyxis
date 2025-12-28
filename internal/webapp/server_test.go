package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/zrougamed/pyxis/internal/k8s"
)

type fakeClusterClient struct {
	currentContext string
	serverVersion  string
	contexts       []string
	namespaces     []string
	pods           []k8s.PodInfo
	resources      map[string][]k8s.ResourceInfo
	details        map[string]*k8s.CompactManifest
	env            []k8s.ContainerInfo
	logs           string
	switchedTo     string
}

func (f *fakeClusterClient) CurrentContext() string         { return f.currentContext }
func (f *fakeClusterClient) ServerVersion() (string, error) { return f.serverVersion, nil }
func (f *fakeClusterClient) Contexts() []string             { return f.contexts }
func (f *fakeClusterClient) SwitchContext(ctx string) error {
	f.currentContext = ctx
	f.switchedTo = ctx
	return nil
}
func (f *fakeClusterClient) Namespaces(context.Context) ([]string, error) { return f.namespaces, nil }
func (f *fakeClusterClient) ListPods(context.Context, string, k8s.PodPhaseFilter) ([]k8s.PodInfo, error) {
	return f.pods, nil
}
func (f *fakeClusterClient) GetPodLogs(context.Context, string, string, string, int64, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.logs)), nil
}
func (f *fakeClusterClient) GetPodEnvVars(context.Context, string, string) ([]k8s.ContainerInfo, error) {
	return f.env, nil
}
func (f *fakeClusterClient) ListDeployments(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["deployments"], nil
}
func (f *fakeClusterClient) ListServices(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["services"], nil
}
func (f *fakeClusterClient) ListNodes(context.Context) ([]k8s.ResourceInfo, error) {
	return f.resources["nodes"], nil
}
func (f *fakeClusterClient) ListEvents(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["events"], nil
}
func (f *fakeClusterClient) ListConfigMaps(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["configmaps"], nil
}
func (f *fakeClusterClient) ListSecrets(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["secrets"], nil
}
func (f *fakeClusterClient) ListStatefulSets(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["statefulsets"], nil
}
func (f *fakeClusterClient) ListDaemonSets(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["daemonsets"], nil
}
func (f *fakeClusterClient) ListJobs(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["jobs"], nil
}
func (f *fakeClusterClient) ListCronJobs(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["cronjobs"], nil
}
func (f *fakeClusterClient) ListIngresses(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["ingresses"], nil
}
func (f *fakeClusterClient) ListPersistentVolumeClaims(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["pvcs"], nil
}
func (f *fakeClusterClient) ListPersistentVolumes(context.Context) ([]k8s.ResourceInfo, error) {
	return f.resources["pvs"], nil
}
func (f *fakeClusterClient) ListNamespaceInfos(context.Context) ([]k8s.ResourceInfo, error) {
	return f.resources["namespaces"], nil
}
func (f *fakeClusterClient) ListHPAs(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["hpas"], nil
}
func (f *fakeClusterClient) ListCRDs(context.Context) ([]k8s.ResourceInfo, error) {
	return f.resources["crds"], nil
}
func (f *fakeClusterClient) ListHelmReleases(context.Context, string) ([]k8s.ResourceInfo, error) {
	return f.resources["helm"], nil
}
func (f *fakeClusterClient) GetPodContainers(context.Context, string, string) ([]string, error) {
	return []string{"api"}, nil
}
func (f *fakeClusterClient) DeletePod(context.Context, string, string) error { return nil }
func (f *fakeClusterClient) DeleteDeployment(context.Context, string, string) error {
	return nil
}
func (f *fakeClusterClient) CreateNamespace(context.Context, string) error { return nil }
func (f *fakeClusterClient) DeleteNamespace(context.Context, string) error { return nil }
func (f *fakeClusterClient) ScaleDeployment(context.Context, string, string, int32) error {
	return nil
}
func (f *fakeClusterClient) ScaleStatefulSet(context.Context, string, string, int32) error {
	return nil
}
func (f *fakeClusterClient) RestartDeployment(context.Context, string, string) error { return nil }
func (f *fakeClusterClient) RestartStatefulSet(context.Context, string, string) error {
	return nil
}
func (f *fakeClusterClient) RestartDaemonSet(context.Context, string, string) error { return nil }
func (f *fakeClusterClient) ExecInPod(context.Context, string, string, string, []string, io.Writer, io.Writer, io.Reader) error {
	return nil
}
func (f *fakeClusterClient) StartPortForward(context.Context, string, string, int, int) (func(), error) {
	return func() {}, nil
}
func (f *fakeClusterClient) GetPodCompact(context.Context, string, string) (*k8s.CompactManifest, error) {
	return f.details["Pod"], nil
}
func (f *fakeClusterClient) GetDeploymentCompact(context.Context, string, string) (*k8s.CompactManifest, error) {
	return f.details["Deployment"], nil
}
func (f *fakeClusterClient) GetStatefulSetCompact(context.Context, string, string) (*k8s.CompactManifest, error) {
	return f.details["StatefulSet"], nil
}
func (f *fakeClusterClient) GetDaemonSetCompact(context.Context, string, string) (*k8s.CompactManifest, error) {
	return f.details["DaemonSet"], nil
}
func (f *fakeClusterClient) GetConfigMapData(context.Context, string, string) (*k8s.CompactManifest, error) {
	return f.details["ConfigMap"], nil
}
func (f *fakeClusterClient) GetSecretData(context.Context, string, string) (*k8s.CompactManifest, error) {
	return f.details["Secret"], nil
}
func (f *fakeClusterClient) GetResourceYAML(_ context.Context, kind, namespace, name string) (string, error) {
	if d := f.details[kind]; d != nil {
		return d.Content, nil
	}
	return fmt.Sprintf("apiVersion: v1\nkind: %s\nmetadata:\n  name: %s\n  namespace: %s\n", kind, name, namespace), nil
}
func (f *fakeClusterClient) ApplyYAML(context.Context, string) (string, error) {
	return "Applied", nil
}
func (f *fakeClusterClient) LintYAML(context.Context, string, bool) ([]k8s.LintIssue, error) {
	return nil, nil
}

func newTestServer(t *testing.T) (*Server, *fakeClusterClient) {
	t.Helper()
	client := &fakeClusterClient{
		currentContext: "kind-dev",
		serverVersion:  "v1.32.0",
		contexts:       []string{"kind-dev", "prod"},
		namespaces:     []string{"default", "kube-system"},
		pods: []k8s.PodInfo{{
			Name:      "api-0",
			Namespace: "default",
			Phase:     corev1.PodRunning,
			Ready:     "1/1",
			Restarts:  0,
			Node:      "worker-a",
			Images:    []string{"ghcr.io/example/api:1.0.0"},
		}},
		resources: map[string][]k8s.ResourceInfo{
			"deployments":  {{Kind: "Deployment", Name: "api", Namespace: "default", Status: "1/1 ready"}},
			"services":     {{Kind: "Service", Name: "api", Namespace: "default", Status: "ClusterIP"}},
			"nodes":        {{Kind: "Node", Name: "worker-a", Status: "Ready"}},
			"events":       {{Kind: "Event", Name: "api-0", Namespace: "default", Status: "Warning"}},
			"configmaps":   {{Kind: "ConfigMap", Name: "app-config", Namespace: "default"}},
			"secrets":      {{Kind: "Secret", Name: "app-secret", Namespace: "default", Status: "Opaque"}},
			"statefulsets": {{Kind: "StatefulSet", Name: "db", Namespace: "default", Status: "1/1 ready"}},
			"daemonsets":   {{Kind: "DaemonSet", Name: "node-exporter", Namespace: "default", Status: "1 desired, 1 ready"}},
		},
		details: map[string]*k8s.CompactManifest{
			"Pod":         {Kind: "Pod", Namespace: "default", Name: "api-0", Content: "pod compact"},
			"Deployment":  {Kind: "Deployment", Namespace: "default", Name: "api", Content: "deployment compact"},
			"StatefulSet": {Kind: "StatefulSet", Namespace: "default", Name: "db", Content: "statefulset compact"},
			"DaemonSet":   {Kind: "DaemonSet", Namespace: "default", Name: "node-exporter", Content: "daemonset compact"},
			"ConfigMap":   {Kind: "ConfigMap", Namespace: "default", Name: "app-config", Content: "configmap content"},
			"Secret":      {Kind: "Secret", Namespace: "default", Name: "app-secret", Content: "secret content"},
		},
		env: []k8s.ContainerInfo{{
			Name:    "api",
			Image:   "ghcr.io/example/api:1.0.0",
			EnvVars: []k8s.EnvVar{{Name: "LOG_LEVEL", Value: "debug"}},
		}},
		logs: "line 1\nline 2\n",
	}
	server, err := NewServer(Config{CookieSecret: "test-secret", BaseURL: "http://localhost:8080"}, client)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return server, client
}

func authCookie(t *testing.T, server *Server) *http.Cookie {
	t.Helper()
	value, err := server.encodeSession(session{Name: "Dex User", Email: "dex@example.com", Expiry: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("encodeSession() error = %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: value, Path: "/"}
}

func TestRequireAuthReturnsUnauthorizedWithoutSession(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAPIEndpointsReturnDataWithSession(t *testing.T) {
	server, client := newTestServer(t)
	mux := server.Handler()

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "me", path: "/api/me", want: "dex@example.com"},
		{name: "summary", path: "/api/summary", want: "kind-dev"},
		{name: "namespaces", path: "/api/namespaces", want: "kube-system"},
		{name: "contexts", path: "/api/contexts", want: "prod"},
		{name: "pods", path: "/api/pods?filter=running", want: "api-0"},
		{name: "pod logs", path: "/api/pod-logs?namespace=default&pod=api-0", want: "line 1"},
		{name: "pod env", path: "/api/pod-env?namespace=default&pod=api-0", want: "LOG_LEVEL"},
		{name: "resources", path: "/api/resources?view=deployments&namespace=default", want: "1/1 ready"},
		{name: "detail", path: "/api/detail?kind=Deployment&namespace=default&name=api", want: "deployment compact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.AddCookie(authCookie(t, server))
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}
			if body := rr.Body.String(); !strings.Contains(body, tc.want) {
				t.Fatalf("response %q does not contain %q", body, tc.want)
			}
		})
	}

	req := httptest.NewRequest(http.MethodPost, "/api/context", strings.NewReader(`{"context":"prod"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(authCookie(t, server))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("switch context status = %d, want %d", rr.Code, http.StatusOK)
	}
	if client.switchedTo != "prod" {
		t.Fatalf("switchedTo = %q, want prod", client.switchedTo)
	}
}

func TestStaticIndexServed(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "<div id=\"root\"></div>") {
		t.Fatalf("expected index html, got %q", rr.Body.String())
	}
}

func TestSessionTamperingIsRejected(t *testing.T) {
	server, _ := newTestServer(t)
	valid := authCookie(t, server)
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: valid.Value + "tampered", Path: "/"})
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestParsePodFilterRejectsUnknownValue(t *testing.T) {
	if _, err := parsePodFilter("mystery"); err == nil {
		t.Fatal("expected error for unsupported filter")
	}
}

func TestNoAuthBypassesSession(t *testing.T) {
	client := &fakeClusterClient{
		currentContext: "kind-dev",
		serverVersion:  "v1.32.0",
		contexts:       []string{"kind-dev"},
		namespaces:     []string{"default"},
		resources:      map[string][]k8s.ResourceInfo{},
	}
	server, err := NewServer(Config{NoAuth: true, BaseURL: "http://localhost:8080"}, client)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHealthEndpoint(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var payload map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("status = %q, want ok", payload["status"])
	}
}

func TestRewriteEndpointPublicURL(t *testing.T) {
	got := rewriteEndpointPublicURL("http://dex:5556/dex/auth", "http://localhost:5556/dex")
	if got != "http://localhost:5556/dex/auth" {
		t.Fatalf("rewriteEndpointPublicURL() = %q, want %q", got, "http://localhost:5556/dex/auth")
	}
}
