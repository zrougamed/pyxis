package fuzzy

import (
	"testing"
)

func TestFind_EmptyPattern(t *testing.T) {
	items := []string{"nginx-pod", "redis-cache", "api-server"}
	matches := Find("", items)

	if len(matches) != len(items) {
		t.Errorf("expected %d matches, got %d", len(items), len(matches))
	}
}

func TestFind_ExactMatch(t *testing.T) {
	items := []string{"nginx-pod", "redis-cache", "api-server"}
	matches := Find("nginx-pod", items)

	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].Str != "nginx-pod" {
		t.Errorf("expected first match to be 'nginx-pod', got %q", matches[0].Str)
	}
}

func TestFind_PartialMatch(t *testing.T) {
	items := []string{
		"my-app-nginx-7d4b8c9f-abc12",
		"redis-cache-5f8c3a-xyz99",
		"nginx-ingress-controller-abcde",
	}
	matches := Find("nginx", items)

	if len(matches) < 2 {
		t.Errorf("expected at least 2 matches for 'nginx', got %d", len(matches))
	}

	for _, m := range matches {
		found := false
		for _, item := range items {
			if m.Str == item {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("match %q not in original items", m.Str)
		}
	}
}

func TestFind_NoMatch(t *testing.T) {
	items := []string{"nginx-pod", "redis-cache"}
	matches := Find("zzzzz", items)

	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestFind_CaseInsensitive(t *testing.T) {
	items := []string{"Nginx-Pod", "REDIS-CACHE"}
	matches := Find("nginx", items)

	if len(matches) == 0 {
		t.Fatal("expected case-insensitive match")
	}
}

func TestFind_FuzzyOrdering(t *testing.T) {
	items := []string{
		"something-completely-different",
		"nginx-proxy",
		"nginx",
	}
	matches := Find("nginx", items)

	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(matches))
	}

	// "nginx" (exact) should score higher than "nginx-proxy".
	if matches[0].Str != "nginx" {
		t.Errorf("expected 'nginx' as best match, got %q", matches[0].Str)
	}
}

func TestFind_BoundaryMatching(t *testing.T) {
	items := []string{
		"my-api-gateway-deployment-7abc",
		"some-random-thing",
	}
	matches := Find("api", items)

	if len(matches) == 0 {
		t.Fatal("expected match on word boundary 'api'")
	}
	if matches[0].Str != items[0] {
		t.Errorf("expected boundary match first, got %q", matches[0].Str)
	}
}

func TestScore_EmptyInputs(t *testing.T) {
	if s := score("", "anything"); s <= 0 {
		t.Errorf("empty pattern should match, got score %d", s)
	}
	if s := score("a", ""); s != 0 {
		t.Errorf("empty text should not match, got score %d", s)
	}
}
