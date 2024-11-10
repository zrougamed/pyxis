package clipboard

import (
	"testing"
)

func TestCopyAndGet(t *testing.T) {
	text := "hello world"
	// Copy may fail if no clipboard tool is available, but internal buffer should work.
	_ = Copy(text)

	got := Get()
	if got != text {
		t.Errorf("expected %q, got %q", text, got)
	}
}

func TestCopyOverwrites(t *testing.T) {
	_ = Copy("first")
	_ = Copy("second")

	got := Get()
	if got != "second" {
		t.Errorf("expected 'second', got %q", got)
	}
}

func TestGetEmpty(t *testing.T) {
	mu.Lock()
	buffer = ""
	mu.Unlock()

	got := Get()
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
