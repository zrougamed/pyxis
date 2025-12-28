package webapp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/zrougamed/pyxis/internal/k8s"
)

const (
	sessionCookieName = "pyxis_session"
	stateCookieName   = "pyxis_state"
)

//go:embed static/*
var staticFS embed.FS

// ClusterClient captures the Kubernetes operations used by the web UI.
type ClusterClient interface {
	CurrentContext() string
	ServerVersion() (string, error)
	Contexts() []string
	SwitchContext(ctx string) error
	Namespaces(ctx context.Context) ([]string, error)
	ListPods(ctx context.Context, namespace string, filter k8s.PodPhaseFilter) ([]k8s.PodInfo, error)
	GetPodLogs(ctx context.Context, namespace, podName, container string, tailLines int64, follow bool) (io.ReadCloser, error)
	GetPodEnvVars(ctx context.Context, namespace, podName string) ([]k8s.ContainerInfo, error)
	GetPodContainers(ctx context.Context, namespace, name string) ([]string, error)
	ListDeployments(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListServices(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListNodes(ctx context.Context) ([]k8s.ResourceInfo, error)
	ListEvents(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListConfigMaps(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListSecrets(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListStatefulSets(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListDaemonSets(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListJobs(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListCronJobs(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListIngresses(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListPersistentVolumeClaims(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListPersistentVolumes(ctx context.Context) ([]k8s.ResourceInfo, error)
	ListNamespaceInfos(ctx context.Context) ([]k8s.ResourceInfo, error)
	ListHPAs(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	ListCRDs(ctx context.Context) ([]k8s.ResourceInfo, error)
	ListHelmReleases(ctx context.Context, namespace string) ([]k8s.ResourceInfo, error)
	GetPodCompact(ctx context.Context, namespace, name string) (*k8s.CompactManifest, error)
	GetDeploymentCompact(ctx context.Context, namespace, name string) (*k8s.CompactManifest, error)
	GetStatefulSetCompact(ctx context.Context, namespace, name string) (*k8s.CompactManifest, error)
	GetDaemonSetCompact(ctx context.Context, namespace, name string) (*k8s.CompactManifest, error)
	GetConfigMapData(ctx context.Context, namespace, name string) (*k8s.CompactManifest, error)
	GetSecretData(ctx context.Context, namespace, name string) (*k8s.CompactManifest, error)
	GetResourceYAML(ctx context.Context, kind, namespace, name string) (string, error)
	ApplyYAML(ctx context.Context, manifest string) (string, error)
	LintYAML(ctx context.Context, manifest string, dryRun bool) ([]k8s.LintIssue, error)
	CreateNamespace(ctx context.Context, name string) error
	DeleteNamespace(ctx context.Context, name string) error
	DeletePod(ctx context.Context, namespace, name string) error
	DeleteDeployment(ctx context.Context, namespace, name string) error
	ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error
	ScaleStatefulSet(ctx context.Context, namespace, name string, replicas int32) error
	RestartDeployment(ctx context.Context, namespace, name string) error
	RestartStatefulSet(ctx context.Context, namespace, name string) error
	RestartDaemonSet(ctx context.Context, namespace, name string) error
	ExecInPod(ctx context.Context, namespace, pod, container string, command []string, stdout, stderr io.Writer, stdin io.Reader) error
	StartPortForward(ctx context.Context, namespace, pod string, localPort, remotePort int) (func(), error)
}

// Config holds the web server and Dex integration settings.
type Config struct {
	ListenAddr      string
	BaseURL         string
	DexIssuer       string
	DexClientID     string
	DexClientSecret string
	DexPublicIssuer string
	CookieSecret    string
	Scopes          []string
	NoAuth          bool // skip session auth (local/dev use)
}

// Server serves the React app, JSON API, and Dex auth flow.
type Server struct {
	cfg          Config
	client       ClusterClient
	oauthConfig  *oauth2.Config
	userinfoURL  string
	httpClient   *http.Client
	cookieSecret []byte
	static       fs.FS
}

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

type userInfo struct {
	Subject string `json:"sub"`
	Name    string `json:"name"`
	Email   string `json:"email"`
}

type session struct {
	Subject     string    `json:"sub"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	AccessToken string    `json:"access_token"`
	Expiry      time.Time `json:"expiry"`
}

type apiError struct {
	Error    string `json:"error"`
	LoginURL string `json:"loginUrl,omitempty"`
}

type summaryResponse struct {
	CurrentContext string `json:"currentContext"`
	ServerVersion  string `json:"serverVersion"`
}

type meResponse struct {
	Authenticated bool   `json:"authenticated"`
	Name          string `json:"name"`
	Email         string `json:"email"`
}

type switchContextRequest struct {
	Context string `json:"context"`
}

type detailResponse struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Title     string `json:"title"`
	Content   string `json:"content"`
}

// NewServer creates a new web server instance.
func NewServer(cfg Config, client ClusterClient) (*Server, error) {
	if client == nil {
		return nil, errors.New("cluster client is required")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	if cfg.NoAuth && cfg.DexIssuer != "" {
		return nil, errors.New("cannot enable Dex authentication together with --no-auth")
	}
	if !cfg.NoAuth && cfg.CookieSecret == "" {
		return nil, errors.New("cookie secret is required (or pass --no-auth)")
	}
	if cfg.CookieSecret == "" {
		cfg.CookieSecret = "pyxis-no-auth"
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}

	server := &Server{
		cfg:          cfg,
		client:       client,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		cookieSecret: []byte(cfg.CookieSecret),
	}

	staticRoot, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("loading embedded UI assets: %w", err)
	}
	server.static = staticRoot

	if cfg.DexIssuer != "" {
		if cfg.DexClientID == "" || cfg.DexClientSecret == "" {
			return nil, errors.New("Dex client id and secret are required when Dex issuer is configured")
		}
		discovery, err := server.discoverOIDC(context.Background(), cfg.DexIssuer)
		if err != nil {
			return nil, err
		}
		server.userinfoURL = discovery.UserinfoEndpoint
		authURL := discovery.AuthorizationEndpoint
		if cfg.DexPublicIssuer != "" {
			authURL = rewriteEndpointPublicURL(discovery.AuthorizationEndpoint, cfg.DexPublicIssuer)
		}
		server.oauthConfig = &oauth2.Config{
			ClientID:     cfg.DexClientID,
			ClientSecret: cfg.DexClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  authURL,
				TokenURL: discovery.TokenEndpoint,
			},
			RedirectURL: strings.TrimRight(cfg.BaseURL, "/") + "/oauth/callback",
			Scopes:      cfg.Scopes,
		}
	}

	return server, nil
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/oauth/callback", s.handleCallback)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.Handle("/api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("/api/summary", s.requireAuth(http.HandlerFunc(s.handleSummary)))
	mux.Handle("/api/contexts", s.requireAuth(http.HandlerFunc(s.handleContexts)))
	mux.Handle("/api/context", s.requireAuth(http.HandlerFunc(s.handleContextSwitch)))
	mux.Handle("/api/namespaces", s.requireAuth(http.HandlerFunc(s.handleNamespaces)))
	mux.Handle("/api/pods", s.requireAuth(http.HandlerFunc(s.handlePods)))
	mux.Handle("/api/pod-logs", s.requireAuth(http.HandlerFunc(s.handlePodLogs)))
	mux.Handle("/api/pod-env", s.requireAuth(http.HandlerFunc(s.handlePodEnv)))
	mux.Handle("/api/pod-containers", s.requireAuth(http.HandlerFunc(s.handlePodContainers)))
	mux.Handle("/api/resources", s.requireAuth(http.HandlerFunc(s.handleResources)))
	mux.Handle("/api/detail", s.requireAuth(http.HandlerFunc(s.handleDetail)))
	mux.Handle("/api/yaml/apply", s.requireAuth(http.HandlerFunc(s.handleYAMLApply)))
	mux.Handle("/api/yaml/lint", s.requireAuth(http.HandlerFunc(s.handleYAMLLint)))
	mux.Handle("/api/actions", s.requireAuth(http.HandlerFunc(s.handleActions)))
	mux.Handle("/api/exec/ws", s.requireAuth(http.HandlerFunc(s.handleExecWS)))
	mux.Handle("/", s.staticHandler())
	return mux
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.cfg.ListenAddr, s.Handler())
}

func (s *Server) staticHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "." || clean == "" {
			http.ServeFileFS(w, r, s.static, "index.html")
			return
		}
		if _, err := fs.Stat(s.static, clean); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFileFS(w, r, s.static, "index.html")
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.oauthConfig == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, apiError{Error: "Dex authentication is not configured"})
		return
	}
	state, err := randomToken(32)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, apiError{Error: "failed to create auth state"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: state, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(s.cfg.BaseURL), MaxAge: 300})
	http.Redirect(w, r, s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline), http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	if s.oauthConfig == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, apiError{Error: "Dex authentication is not configured"})
		return
	}
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || r.URL.Query().Get("state") == "" || stateCookie.Value != r.URL.Query().Get("state") {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid auth state"})
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "missing authorization code"})
		return
	}
	token, err := s.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		s.writeJSON(w, http.StatusUnauthorized, apiError{Error: "failed to exchange authorization code"})
		return
	}
	profile, err := s.fetchUserInfo(r.Context(), token.AccessToken)
	if err != nil {
		s.writeJSON(w, http.StatusUnauthorized, apiError{Error: "failed to fetch user profile from Dex"})
		return
	}
	expiresAt := token.Expiry
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(1 * time.Hour)
	}
	encoded, err := s.encodeSession(session{Subject: profile.Subject, Name: profile.Name, Email: profile.Email, AccessToken: token.AccessToken, Expiry: expiresAt})
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, apiError{Error: "failed to persist session"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: encoded, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(s.cfg.BaseURL), Expires: expiresAt})
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(s.cfg.BaseURL)})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(s.cfg.BaseURL)})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFromContext(r.Context())
	s.writeJSON(w, http.StatusOK, meResponse{Authenticated: true, Name: sess.Name, Email: sess.Email})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	serverVersion, err := s.client.ServerVersion()
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, summaryResponse{CurrentContext: s.client.CurrentContext(), ServerVersion: serverVersion})
}

func (s *Server) handleContexts(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string][]string{"items": s.client.Contexts()})
}

func (s *Server) handleContextSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed"})
		return
	}
	var req switchContextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
		return
	}
	ctxName := strings.TrimSpace(req.Context)
	if ctxName == "" {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "context is required"})
		return
	}
	if err := s.client.SwitchContext(ctxName); err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Switched to context: %s", ctxName)})
}

func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := s.client.Namespaces(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string][]string{"items": namespaces})
}

func (s *Server) handlePods(w http.ResponseWriter, r *http.Request) {
	filter, err := parsePodFilter(r.URL.Query().Get("filter"))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: err.Error()})
		return
	}
	pods, err := s.client.ListPods(r.Context(), r.URL.Query().Get("namespace"), filter)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string][]k8s.PodInfo{"items": pods})
}

func (s *Server) handlePodLogs(w http.ResponseWriter, r *http.Request) {
	namespace, podName, ok := podParams(r)
	if !ok {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "namespace and pod are required"})
		return
	}
	container := strings.TrimSpace(r.URL.Query().Get("container"))
	follow := r.URL.Query().Get("follow") == "true" || r.URL.Query().Get("follow") == "1"
	tailLines := int64(100)
	if raw := strings.TrimSpace(r.URL.Query().Get("tail")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			tailLines = n
		}
	}

	stream, err := s.client.GetPodLogs(r.Context(), namespace, podName, container, tailLines, follow)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	defer stream.Close()

	if follow {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := stream.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return
				}
				if canFlush {
					flusher.Flush()
				}
			}
			if readErr != nil {
				return
			}
		}
	}

	data, err := io.ReadAll(stream)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	title := fmt.Sprintf("Logs: %s/%s", namespace, podName)
	if container != "" {
		title = fmt.Sprintf("Logs: %s/%s [%s]", namespace, podName, container)
	}
	s.writeJSON(w, http.StatusOK, detailResponse{Kind: "PodLogs", Namespace: namespace, Name: podName, Title: title, Content: string(data)})
}

func (s *Server) handlePodContainers(w http.ResponseWriter, r *http.Request) {
	namespace, podName, ok := podParams(r)
	if !ok {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "namespace and pod are required"})
		return
	}
	names, err := s.client.GetPodContainers(r.Context(), namespace, podName)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": names})
}

func (s *Server) handlePodEnv(w http.ResponseWriter, r *http.Request) {
	namespace, podName, ok := podParams(r)
	if !ok {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "namespace and pod are required"})
		return
	}
	containers, err := s.client.GetPodEnvVars(r.Context(), namespace, podName)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"title": fmt.Sprintf("Env Vars: %s/%s", namespace, podName), "items": containers})
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	namespace := r.URL.Query().Get("namespace")
	var (
		items []k8s.ResourceInfo
		err   error
	)

	switch view {
	case "deployments":
		items, err = s.client.ListDeployments(r.Context(), namespace)
	case "services":
		items, err = s.client.ListServices(r.Context(), namespace)
	case "nodes":
		items, err = s.client.ListNodes(r.Context())
	case "events":
		items, err = s.client.ListEvents(r.Context(), namespace)
	case "configmaps":
		items, err = s.client.ListConfigMaps(r.Context(), namespace)
	case "secrets":
		items, err = s.client.ListSecrets(r.Context(), namespace)
	case "statefulsets":
		items, err = s.client.ListStatefulSets(r.Context(), namespace)
	case "daemonsets":
		items, err = s.client.ListDaemonSets(r.Context(), namespace)
	case "jobs":
		items, err = s.client.ListJobs(r.Context(), namespace)
	case "cronjobs":
		items, err = s.client.ListCronJobs(r.Context(), namespace)
	case "ingresses":
		items, err = s.client.ListIngresses(r.Context(), namespace)
	case "pvcs":
		items, err = s.client.ListPersistentVolumeClaims(r.Context(), namespace)
	case "pvs":
		items, err = s.client.ListPersistentVolumes(r.Context())
	case "namespaces":
		items, err = s.client.ListNamespaceInfos(r.Context())
	case "hpas":
		items, err = s.client.ListHPAs(r.Context(), namespace)
	case "crds":
		items, err = s.client.ListCRDs(r.Context())
	case "helm":
		items, err = s.client.ListHelmReleases(r.Context(), namespace)
	default:
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: fmt.Sprintf("unsupported resource view %q", view)})
		return
	}
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"view": view, "items": items})
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	namespace := strings.TrimSpace(r.URL.Query().Get("namespace"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if kind == "" || name == "" {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "kind and name are required"})
		return
	}

	content, err := s.client.GetResourceYAML(r.Context(), kind, namespace, name)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	title := fmt.Sprintf("%s: %s", kind, name)
	if namespace != "" {
		title = fmt.Sprintf("%s: %s/%s", kind, namespace, name)
	}
	s.writeJSON(w, http.StatusOK, detailResponse{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		Title:     title,
		Content:   content,
	})
}

type yamlBody struct {
	Manifest string `json:"manifest"`
	DryRun   bool   `json:"dryRun,omitempty"`
}

func (s *Server) handleYAMLApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed"})
		return
	}
	var body yamlBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
		return
	}
	message, err := s.client.ApplyYAML(r.Context(), body.Manifest)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

func (s *Server) handleYAMLLint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed"})
		return
	}
	var body yamlBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
		return
	}
	issues, err := s.client.LintYAML(r.Context(), body.Manifest, body.DryRun)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"issues": issues})
}

func podParams(r *http.Request) (namespace, podName string, ok bool) {
	namespace = strings.TrimSpace(r.URL.Query().Get("namespace"))
	podName = strings.TrimSpace(r.URL.Query().Get("pod"))
	return namespace, podName, namespace != "" && podName != ""
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.NoAuth {
			sess := session{Name: "local", Email: "local@localhost", Expiry: time.Now().Add(24 * time.Hour)}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, sess)))
			return
		}
		sess, err := s.readSession(r)
		if err != nil {
			loginURL := "/login"
			if s.oauthConfig == nil {
				loginURL = ""
			}
			s.writeJSON(w, http.StatusUnauthorized, apiError{Error: "authentication required", LoginURL: loginURL})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, sess)))
	})
}

type sessionContextKey struct{}

func sessionFromContext(ctx context.Context) (session, bool) {
	sess, ok := ctx.Value(sessionContextKey{}).(session)
	return sess, ok
}

func (s *Server) readSession(r *http.Request) (session, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return session{}, err
	}
	sess, err := s.decodeSession(cookie.Value)
	if err != nil {
		return session{}, err
	}
	if time.Now().After(sess.Expiry) {
		return session{}, errors.New("session expired")
	}
	return sess, nil
}

func (s *Server) encodeSession(sess session) (string, error) {
	payload, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.cookieSecret)
	mac.Write(payload)
	signature := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + hex.EncodeToString(signature), nil
}

func (s *Server) decodeSession(value string) (session, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return session{}, errors.New("invalid session format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return session{}, err
	}
	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return session{}, err
	}
	mac := hmac.New(sha256.New, s.cookieSecret)
	mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return session{}, errors.New("invalid session signature")
	}
	var sess session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return session{}, err
	}
	return sess, nil
}

func (s *Server) discoverOIDC(ctx context.Context, issuer string) (oidcDiscovery, error) {
	wellKnown := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return oidcDiscovery{}, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return oidcDiscovery{}, fmt.Errorf("discovering Dex endpoints: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return oidcDiscovery{}, fmt.Errorf("discovering Dex endpoints: unexpected status %s", resp.Status)
	}
	var discovery oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return oidcDiscovery{}, fmt.Errorf("parsing Dex discovery document: %w", err)
	}
	if discovery.AuthorizationEndpoint == "" || discovery.TokenEndpoint == "" || discovery.UserinfoEndpoint == "" {
		return oidcDiscovery{}, errors.New("Dex discovery document is missing required endpoints")
	}
	return discovery, nil
}

func (s *Server) fetchUserInfo(ctx context.Context, accessToken string) (userInfo, error) {
	if s.userinfoURL == "" {
		return userInfo{}, errors.New("Dex userinfo endpoint is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.userinfoURL, nil)
	if err != nil {
		return userInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return userInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return userInfo{}, fmt.Errorf("userinfo request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var profile userInfo
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return userInfo{}, err
	}
	return profile, nil
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parsePodFilter(raw string) (k8s.PodPhaseFilter, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return k8s.PodFilterAll, nil
	case "running":
		return k8s.PodFilterRunning, nil
	case "not-running":
		return k8s.PodFilterNotRunning, nil
	case "failed":
		return k8s.PodFilterFailed, nil
	case "pending":
		return k8s.PodFilterPending, nil
	case "succeeded":
		return k8s.PodFilterSucceeded, nil
	default:
		return k8s.PodFilterAll, fmt.Errorf("unsupported pod filter %q", raw)
	}
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func rewriteEndpointPublicURL(endpointURL, publicIssuer string) string {
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return endpointURL
	}
	publicURL, err := url.Parse(publicIssuer)
	if err != nil {
		return endpointURL
	}
	publicURL.Path = endpoint.Path
	publicURL.RawPath = endpoint.RawPath
	publicURL.RawQuery = endpoint.RawQuery
	publicURL.Fragment = endpoint.Fragment
	return publicURL.String()
}

func isHTTPS(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https"
}
