package windsurfmeta

import "testing"

func TestCanonicalNameMapsLegacyAliases(t *testing.T) {
	tests := map[string]string{
		"gpt4o":                   "gpt-4o",
		"claude-3-5-sonnet":       "claude-3.5-sonnet",
		"gpt4-o3-mini":            "o3-mini",
		"claude-3-7-sonnet-think": "claude-3.7-sonnet-thinking",
	}

	for input, expected := range tests {
		if got := CanonicalName(input); got != expected {
			t.Fatalf("expected %q -> %q, got %q", input, expected, got)
		}
	}
}

func TestNormalizeRegistryKeepsCanonicalKeys(t *testing.T) {
	registry := NormalizeRegistry(map[string]string{
		"gpt4o":               "109",
		"claude-3-7-sonnet":   "226",
		"custom-future-model": "999",
	})

	if _, ok := registry["gpt4o"]; ok {
		t.Fatalf("legacy alias should not remain in normalized registry")
	}
	if got := registry["gpt-4o"]; got != "109" {
		t.Fatalf("expected canonical gpt-4o mapping, got %q", got)
	}
	if got := registry["claude-3.7-sonnet"]; got != "226" {
		t.Fatalf("expected canonical claude-3.7-sonnet mapping, got %q", got)
	}
	if got := registry["custom-future-model"]; got != "999" {
		t.Fatalf("expected custom model to survive normalization, got %q", got)
	}
}
