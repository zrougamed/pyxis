package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/zrougamed/pyxis/internal/k8s"
)

var execUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// PodTTYStreamer is implemented by clients that can open an interactive pod shell.
type PodTTYStreamer interface {
	StreamPodExecTTY(ctx context.Context, namespace, pod, container string, command []string, stdin io.Reader, stdout io.Writer, resizeQueue remotecommand.TerminalSizeQueue) error
}

type resizeQueue struct {
	ch   chan remotecommand.TerminalSize
	once sync.Once
}

func newResizeQueue() *resizeQueue {
	return &resizeQueue{ch: make(chan remotecommand.TerminalSize, 4)}
}

func (q *resizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return &size
}

func (q *resizeQueue) Push(cols, rows uint16) {
	select {
	case q.ch <- remotecommand.TerminalSize{Width: cols, Height: rows}:
	default:
	}
}

func (q *resizeQueue) Close() {
	q.once.Do(func() { close(q.ch) })
}

type wsWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *wsWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

type wsReader struct {
	conn   *websocket.Conn
	resize *resizeQueue
	buf    []byte
}

func (r *wsReader) Read(p []byte) (int, error) {
	for {
		if len(r.buf) > 0 {
			n := copy(p, r.buf)
			r.buf = r.buf[n:]
			return n, nil
		}
		msgType, data, err := r.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if msgType == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
			var payload struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(data, &payload) == nil && payload.Type == "resize" {
				if r.resize != nil && payload.Cols > 0 && payload.Rows > 0 {
					r.resize.Push(payload.Cols, payload.Rows)
				}
				continue
			}
		}
		r.buf = data
	}
}

func (s *Server) handleExecWS(w http.ResponseWriter, r *http.Request) {
	streamer, ok := s.client.(PodTTYStreamer)
	if !ok {
		// Allow concrete *k8s.Client even if wrapped oddly.
		if kc, castOK := s.client.(*k8s.Client); castOK {
			streamer = kc
			ok = true
		}
	}
	if !ok {
		s.writeJSON(w, http.StatusNotImplemented, apiError{Error: "interactive exec is not available with this client"})
		return
	}

	namespace := strings.TrimSpace(r.URL.Query().Get("namespace"))
	pod := strings.TrimSpace(r.URL.Query().Get("pod"))
	container := strings.TrimSpace(r.URL.Query().Get("container"))
	if namespace == "" || pod == "" {
		s.writeJSON(w, http.StatusBadRequest, apiError{Error: "namespace and pod are required"})
		return
	}

	command := []string{"/bin/sh", "-c", k8s.PodShellCommand}
	if raw := strings.TrimSpace(r.URL.Query().Get("command")); raw != "" {
		command = []string{"/bin/sh", "-c", raw}
	}

	conn, err := execUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	resize := newResizeQueue()
	defer resize.Close()

	stdin := &wsReader{conn: conn, resize: resize}
	stdout := &wsWriter{conn: conn}

	_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("connected to %s/%s\r\n", namespace, pod)))

	err = streamer.StreamPodExecTTY(ctx, namespace, pod, container, command, stdin, stdout, resize)
	if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[exec ended: "+err.Error()+"]\r\n"))
	}
}
