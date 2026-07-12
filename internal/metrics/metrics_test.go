package metrics

import "testing"

func TestShortID(t *testing.T) {
	if got := shortID("abcdef1234567890abcdef"); got != "abcdef123456" {
		t.Fatalf("expected 12-char short id, got %q", got)
	}
	if got := shortID("short"); got != "short" {
		t.Fatalf("expected unchanged id, got %q", got)
	}
	if got := shortID(""); got != "" {
		t.Fatalf("expected empty id, got %q", got)
	}
}
