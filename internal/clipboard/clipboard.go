// Package clipboard provides a portable clipboard abstraction.
//
// Priority order:
//  1. OSC 52 (for SSH/local terminal clipboard)
//  2. Native OS clipboard tools
//  3. Internal in-process buffer fallback
//
// Notes:
// - OSC 52 works only if the terminal emulator supports it.
// - tmux passthrough is supported.
// - Internal buffer is process-local only.
package clipboard

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var (
	mu     sync.Mutex
	buffer string
)

// Copy copies text using the best available method.
// It always stores the text in the internal buffer first.
func Copy(text string) error {
	mu.Lock()
	buffer = text
	mu.Unlock()

	// 1) Prefer terminal clipboard via OSC 52 when stdout looks like a terminal session.
	if canUseOSC52() {
		if err := copyOSC52(text); err == nil {
			return nil
		}
	}

	// 2) Fallback to native OS clipboard on the machine running the program.
	if err := copyNative(text); err == nil {
		return nil
	}

	// 3) Internal buffer fallback already stored above.
	return nil
}

// Get returns the last copied text from the internal in-process buffer.
func Get() string {
	mu.Lock()
	defer mu.Unlock()
	return buffer
}

// copyOSC52 emits an OSC 52 sequence to stdout.
// If running inside tmux, it wraps the sequence for passthrough.
func copyOSC52(text string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	seq := osc52Sequence(encoded)

	_, err := os.Stdout.WriteString(seq)
	if err != nil {
		return err
	}
	var w io.Writer = os.Stdout
	// Best effort flush for terminals that react immediately.
	if f, ok := w.(*os.File); ok {
		_ = f.Sync()
	}
	return nil
}

// osc52Sequence builds the proper OSC 52 escape sequence.
// Inside tmux, OSC must be wrapped so tmux passes it through.
func osc52Sequence(b64 string) string {
	raw := "\x1b]52;c;" + b64 + "\a"

	// tmux passthrough wrapper
	// tmux expects:
	// ESC P tmux; ESC <payload with ESC doubled> ESC \
	if os.Getenv("TMUX") != "" {
		escaped := strings.ReplaceAll(raw, "\x1b", "\x1b\x1b")
		return "\x1bPtmux;" + escaped + "\x1b\\"
	}

	return raw
}

// canUseOSC52 decides whether terminal clipboard is worth trying.
func canUseOSC52() bool {
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}

	// Strong hints that we're in a remote or terminal-driven session.
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return true
	}

	// Also allow local terminal use.
	return true
}

// copyNative attempts native clipboard commands on the current host OS.
func copyNative(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err == nil {
			cmd = exec.Command("pbcopy")
		}

	case "linux":
		// Wayland first if present.
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("wl-copy"); err == nil {
				cmd = exec.Command("wl-copy")
				break
			}
		}

		// X11 tools require DISPLAY.
		if os.Getenv("DISPLAY") != "" {
			if _, err := exec.LookPath("xclip"); err == nil {
				cmd = exec.Command("xclip", "-selection", "clipboard")
			} else if _, err := exec.LookPath("xsel"); err == nil {
				cmd = exec.Command("xsel", "--clipboard", "--input")
			}
		}

	case "windows":
		// "clip" is usually used via cmd /c clip
		if _, err := exec.LookPath("cmd"); err == nil {
			cmd = exec.Command("cmd", "/c", "clip")
		}
	}

	if cmd == nil {
		return fmt.Errorf("no native clipboard command available")
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
