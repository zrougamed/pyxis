package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

// DeleteDeployment deletes a deployment.
func (c *Client) DeleteDeployment(ctx context.Context, namespace, name string) error {
	return c.clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ScaleStatefulSet scales a statefulset to the given replica count.
func (c *Client) ScaleStatefulSet(ctx context.Context, namespace, name string, replicas int32) error {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting statefulset: %w", err)
	}
	sts.Spec.Replicas = &replicas
	_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("scaling statefulset: %w", err)
	}
	return nil
}

// RestartStatefulSet triggers a rolling restart by patching the restartedAt annotation.
func (c *Client) RestartStatefulSet(ctx context.Context, namespace, name string) error {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting statefulset: %w", err)
	}

	if sts.Spec.Template.ObjectMeta.Annotations == nil {
		sts.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	sts.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
	return err
}

// RestartDaemonSet triggers a rolling restart by patching the restartedAt annotation.
func (c *Client) RestartDaemonSet(ctx context.Context, namespace, name string) error {
	ds, err := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting daemonset: %w", err)
	}

	if ds.Spec.Template.ObjectMeta.Annotations == nil {
		ds.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	ds.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = c.clientset.AppsV1().DaemonSets(namespace).Update(ctx, ds, metav1.UpdateOptions{})
	return err
}

// GetPodContainers returns container names from the pod spec (init containers excluded).
func (c *Client) GetPodContainers(ctx context.Context, namespace, name string) ([]string, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod %s/%s: %w", namespace, name, err)
	}
	names := make([]string, len(pod.Spec.Containers))
	for i, ctr := range pod.Spec.Containers {
		names[i] = ctr.Name
	}
	return names, nil
}

// StartPortForward opens a local port-forward to a pod. Returns a stop function.
// Requires rest.Config (nil in fake tests).
func (c *Client) StartPortForward(ctx context.Context, namespace, pod string, localPort, remotePort int) (stop func(), err error) {
	if c.config == nil {
		return nil, fmt.Errorf("port-forward requires REST config")
	}

	reqURL := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("portforward").
		URL()

	transport, upgrader, err := spdy.RoundTripperFor(c.config)
	if err != nil {
		return nil, fmt.Errorf("creating SPDY round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, reqURL)

	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	pf, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	go func() {
		errCh <- pf.ForwardPorts()
	}()

	select {
	case <-readyCh:
		var once sync.Once
		return func() {
			once.Do(func() { close(stopCh) })
		}, nil
	case err := <-errCh:
		return nil, fmt.Errorf("port-forward failed: %w", err)
	case <-ctx.Done():
		close(stopCh)
		return nil, ctx.Err()
	}
}

// ExecInPod runs a one-shot (non-interactive) command in a pod container.
// Requires rest.Config (nil in fake tests).
func (c *Client) ExecInPod(ctx context.Context, namespace, pod, container string, command []string, stdout, stderr io.Writer, stdin io.Reader) error {
	return c.streamExec(ctx, namespace, pod, container, command, false, stdin, stdout, stderr, nil)
}

// PodShellCommand probes common shells then execs the first that exists.
// Important: do not use `exec bash || exec sh` — a failed `exec` can exit
// the process in ash/busybox before the fallback runs.
const PodShellCommand = `for s in /bin/bash /usr/bin/bash /bin/zsh /usr/bin/zsh /bin/ash /usr/bin/ash /bin/dash /usr/bin/dash /bin/ksh /usr/bin/ksh /bin/csh /bin/tcsh /bin/sh /usr/bin/sh; do
  if [ -x "$s" ]; then exec "$s"; fi
done
if command -v bash >/dev/null 2>&1; then exec bash; fi
if command -v ash >/dev/null 2>&1; then exec ash; fi
if command -v sh >/dev/null 2>&1; then exec sh; fi
echo "pyxis: no shell found in container" >&2
exit 127`

// StreamPodExecTTY runs an interactive TTY exec session (for web xterm / OpenLens-style shell).
// resizeQueue may be nil when terminal size is fixed.
func (c *Client) StreamPodExecTTY(ctx context.Context, namespace, pod, container string, command []string, stdin io.Reader, stdout io.Writer, resizeQueue remotecommand.TerminalSizeQueue) error {
	if len(command) == 0 {
		command = []string{"/bin/sh", "-c", PodShellCommand}
	}
	return c.streamExec(ctx, namespace, pod, container, command, true, stdin, stdout, nil, resizeQueue)
}

func (c *Client) streamExec(ctx context.Context, namespace, pod, container string, command []string, tty bool, stdin io.Reader, stdout, stderr io.Writer, resizeQueue remotecommand.TerminalSizeQueue) error {
	if c.config == nil {
		return fmt.Errorf("exec requires REST config")
	}
	if len(command) == 0 {
		return fmt.Errorf("command must not be empty")
	}

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdin:     stdin != nil,
		Stdout:    stdout != nil,
		Stderr:    stderr != nil && !tty,
		TTY:       tty,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.config, http.MethodPost, req.URL())
	if err != nil {
		return fmt.Errorf("creating exec executor: %w", err)
	}

	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		Tty:               tty,
		TerminalSizeQueue: resizeQueue,
	})
}
