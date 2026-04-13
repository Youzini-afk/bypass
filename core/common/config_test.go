package common

import "testing"

func TestIsEnvPlaceholder(t *testing.T) {
	if !IsEnvPlaceholder("${PASSWORD}") {
		t.Fatal("expected ${PASSWORD} to be treated as placeholder")
	}
	if IsEnvPlaceholder("real-password") {
		t.Fatal("did not expect real-password to be treated as placeholder")
	}
}

func TestResolveEnvFallbackPrefersConfiguredValue(t *testing.T) {
	t.Setenv("PASSWORD", "from-env")
	got := ResolveEnvFallback("from-config", "PASSWORD")
	if got != "from-config" {
		t.Fatalf("expected configured value, got %q", got)
	}
}

func TestResolveEnvFallbackUsesEnvWhenPlaceholder(t *testing.T) {
	t.Setenv("PASSWORD", "from-env")
	got := ResolveEnvFallback("${PASSWORD}", "PASSWORD")
	if got != "from-env" {
		t.Fatalf("expected env fallback, got %q", got)
	}
}

func TestResolveEnvFallbackReturnsEmptyWhenPlaceholderIsUnresolved(t *testing.T) {
	t.Setenv("PASSWORD", "")
	got := ResolveEnvFallback("${PASSWORD}", "PASSWORD")
	if got != "" {
		t.Fatalf("expected unresolved placeholder to resolve to empty value, got %q", got)
	}
}
