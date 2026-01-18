package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type actionRequest struct {
	Action     string `json:"action"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Replicas   *int32 `json:"replicas,omitempty"`
	Container  string `json:"container,omitempty"`
	Command    string `json:"command,omitempty"`
	LocalPort  int    `json:"localPort,omitempty"`
	RemotePort int    `json:"remotePort,omitempty"`
}

func (s *Server) handleActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed"})
		return
	}
	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.Kind = strings.TrimSpace(req.Kind)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	if req.Action == "" || req.Name == "" {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "action and name are required"})
		return
	}

	var (
		message string
		err     error
	)
	switch req.Action {
	case "delete":
		message, err = s.actionDelete(r, req)
	case "create":
		message, err = s.actionCreate(r, req)
	case "restart":
		message, err = s.actionRestart(r, req)
	case "scale":
		message, err = s.actionScale(r, req)
	case "exec":
		message, err = s.actionExec(r, req)
	case "portforward":
		message, err = s.actionPortForward(r, req)
	default:
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: fmt.Sprintf("unsupported action %q", req.Action)})
		return
	}
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

func (s *Server) actionCreate(r *http.Request, req actionRequest) (string, error) {
	switch strings.ToLower(req.Kind) {
	case "namespace":
		if err := s.client.CreateNamespace(r.Context(), req.Name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Created namespace %s", req.Name), nil
	default:
		return "", fmt.Errorf("create not supported for kind %q", req.Kind)
	}
}

func (s *Server) actionDelete(r *http.Request, req actionRequest) (string, error) {
	switch strings.ToLower(req.Kind) {
	case "pod", "":
		if err := s.client.DeletePod(r.Context(), req.Namespace, req.Name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Deleted pod %s/%s", req.Namespace, req.Name), nil
	case "deployment":
		if err := s.client.DeleteDeployment(r.Context(), req.Namespace, req.Name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Deleted deployment %s/%s", req.Namespace, req.Name), nil
	case "namespace":
		if err := s.client.DeleteNamespace(r.Context(), req.Name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Deleted namespace %s", req.Name), nil
	default:
		return "", fmt.Errorf("delete not supported for kind %q", req.Kind)
	}
}

func (s *Server) actionRestart(r *http.Request, req actionRequest) (string, error) {
	switch strings.ToLower(req.Kind) {
	case "deployment":
		if err := s.client.RestartDeployment(r.Context(), req.Namespace, req.Name); err != nil {
			return "", err
		}
	case "statefulset":
		if err := s.client.RestartStatefulSet(r.Context(), req.Namespace, req.Name); err != nil {
			return "", err
		}
	case "daemonset":
		if err := s.client.RestartDaemonSet(r.Context(), req.Namespace, req.Name); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("restart not supported for kind %q", req.Kind)
	}
	return fmt.Sprintf("Restarted %s %s/%s", req.Kind, req.Namespace, req.Name), nil
}

func (s *Server) actionScale(r *http.Request, req actionRequest) (string, error) {
	if req.Replicas == nil {
		return "", fmt.Errorf("replicas is required for scale")
	}
	switch strings.ToLower(req.Kind) {
	case "deployment":
		if err := s.client.ScaleDeployment(r.Context(), req.Namespace, req.Name, *req.Replicas); err != nil {
			return "", err
		}
	case "statefulset":
		if err := s.client.ScaleStatefulSet(r.Context(), req.Namespace, req.Name, *req.Replicas); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("scale not supported for kind %q", req.Kind)
	}
	return fmt.Sprintf("Scaled %s %s/%s to %d", req.Kind, req.Namespace, req.Name, *req.Replicas), nil
}

func (s *Server) actionExec(r *http.Request, req actionRequest) (string, error) {
	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		cmd = "sh -c 'hostname; id; pwd'"
	}
	var stdout, stderr bytes.Buffer
	err := s.client.ExecInPod(r.Context(), req.Namespace, req.Name, req.Container, []string{"sh", "-c", cmd}, &stdout, &stderr, nil)
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())
	if err != nil {
		if errOut != "" {
			return "", fmt.Errorf("%w: %s", err, errOut)
		}
		return "", err
	}
	if errOut != "" && out != "" {
		return out + "\n" + errOut, nil
	}
	if errOut != "" {
		return errOut, nil
	}
	if out == "" {
		return "(no output)", nil
	}
	return out, nil
}

func (s *Server) actionPortForward(r *http.Request, req actionRequest) (string, error) {
	local := req.LocalPort
	remote := req.RemotePort
	if remote <= 0 {
		remote = 8080
	}
	if local <= 0 {
		local = remote
	}
	// Detach from the HTTP request so the forward outlives the API call.
	pfCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	stop, err := s.client.StartPortForward(pfCtx, req.Namespace, req.Name, local, remote)
	if err != nil {
		cancel()
		return "", err
	}
	go func() {
		<-pfCtx.Done()
		stop()
		cancel()
	}()
	return fmt.Sprintf("Port-forward started for 30m: localhost:%d → %s/%s:%d", local, req.Namespace, req.Name, remote), nil
}
