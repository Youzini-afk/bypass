package windsurfmeta

import (
	"sort"
	"strings"
)

type OfficialModel struct {
	Name           string `json:"name"`
	Availability   string `json:"availability"`
	Source         string `json:"source"`
	SourceDate     string `json:"sourceDate"`
	Note           string `json:"note,omitempty"`
	BuiltinMapping bool   `json:"builtinMapping"`
}

var defaultRegistry = map[string]string{
	"gpt-4o":                     "109",
	"claude-3.5-sonnet":          "166",
	"gemini-2.0-flash":           "184",
	"deepseek-chat":              "205",
	"deepseek-reasoner":          "206",
	"o3-mini":                    "207",
	"claude-3.7-sonnet":          "226",
	"claude-3.7-sonnet-thinking": "227",
}

var aliases = map[string]string{
	"gpt4o":                      "gpt-4o",
	"claude-3-5-sonnet":          "claude-3.5-sonnet",
	"gpt4-o3-mini":               "o3-mini",
	"gpt-4-o3-mini":              "o3-mini",
	"claude-3-7-sonnet":          "claude-3.7-sonnet",
	"claude-3-7-sonnet-think":    "claude-3.7-sonnet-thinking",
	"claude-3-7-sonnet-thinking": "claude-3.7-sonnet-thinking",
	"claude-3.7-sonnet-think":    "claude-3.7-sonnet-thinking",
}

var officialCatalog = []OfficialModel{
	{Name: "swe-1.5", Availability: "native", Source: "docs", SourceDate: "2026-04-13", Note: "官方模型页当前明确列出为可用模型。"},
	{Name: "swe-1", Availability: "native", Source: "docs", SourceDate: "2026-04-13", Note: "官方模型页当前明确列出为可用模型。"},
	{Name: "swe-grep", Availability: "native", Source: "docs", SourceDate: "2026-04-13", Note: "官方模型页当前明确列出为可用模型。"},
	{Name: "gpt-4.1", Availability: "native", Source: "changelog", SourceDate: "2025-04-14"},
	{Name: "o4-mini-medium", Availability: "native", Source: "changelog", SourceDate: "2025-04-16"},
	{Name: "o4-mini-high", Availability: "native", Source: "changelog", SourceDate: "2025-04-16"},
	{Name: "kimi-k2", Availability: "native", Source: "changelog", SourceDate: "2025-07-23"},
	{Name: "gpt-5-low", Availability: "native", Source: "changelog", SourceDate: "2025-08-12"},
	{Name: "gpt-5-medium", Availability: "native", Source: "changelog", SourceDate: "2025-08-12"},
	{Name: "gpt-5-high", Availability: "native", Source: "changelog", SourceDate: "2025-08-12"},
	{Name: "grok-code-fast-1", Availability: "native", Source: "changelog", SourceDate: "2025-08-25"},
	{Name: "gpt-5-codex", Availability: "native", Source: "changelog", SourceDate: "2025-09-16"},
	{Name: "claude-sonnet-4.5", Availability: "native", Source: "changelog", SourceDate: "2025-09-29"},
	{Name: "gpt-5.1", Availability: "native", Source: "changelog", SourceDate: "2025-11-13"},
	{Name: "gpt-5.1-codex", Availability: "native", Source: "changelog", SourceDate: "2025-11-13"},
	{Name: "gpt-5.1-codex-mini", Availability: "native", Source: "changelog", SourceDate: "2025-11-17"},
	{Name: "claude-opus-4.5", Availability: "native", Source: "changelog", SourceDate: "2025-11-25"},
	{Name: "gpt-5.1-codex-max", Availability: "native", Source: "changelog", SourceDate: "2025-12-04"},
	{Name: "gpt-5.2", Availability: "native", Source: "changelog", SourceDate: "2025-12-11"},
	{Name: "gpt-5.2-codex", Availability: "native", Source: "changelog", SourceDate: "2026-01-14"},
	{Name: "gpt-5.3-codex", Availability: "native", Source: "changelog", SourceDate: "2026-01-22"},
	{Name: "claude-opus-4.6", Availability: "native", Source: "changelog", SourceDate: "2026-02-03"},
	{Name: "claude-opus-4.6-fast", Availability: "native", Source: "changelog", SourceDate: "2026-02-03"},
	{Name: "glm-5", Availability: "native", Source: "changelog", SourceDate: "2026-02-17"},
	{Name: "minimax-m2.5", Availability: "native", Source: "changelog", SourceDate: "2026-02-17"},
	{Name: "claude-sonnet-4.6", Availability: "native", Source: "changelog", SourceDate: "2026-02-17"},
	{Name: "claude-sonnet-4.6-thinking", Availability: "native", Source: "changelog", SourceDate: "2026-02-17"},
	{Name: "gemini-3.1-pro-low", Availability: "native", Source: "changelog", SourceDate: "2026-02-19"},
	{Name: "gemini-3.1-pro-high", Availability: "native", Source: "changelog", SourceDate: "2026-02-19"},
	{Name: "gpt-5.4-low", Availability: "native", Source: "changelog", SourceDate: "2026-03-05"},
	{Name: "gpt-5.4-medium", Availability: "native", Source: "changelog", SourceDate: "2026-03-05"},
	{Name: "gpt-5.4-high", Availability: "native", Source: "changelog", SourceDate: "2026-03-05"},
	{Name: "gpt-5.4-mini", Availability: "native", Source: "changelog", SourceDate: "2026-03-17"},
}

func CanonicalName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	if canonical, ok := aliases[name]; ok {
		return canonical
	}
	return name
}

func DefaultRegistry() map[string]string {
	result := make(map[string]string, len(defaultRegistry))
	for key, value := range defaultRegistry {
		result[key] = value
	}
	return result
}

func DefaultRegistryNames() []string {
	result := make([]string, 0, len(defaultRegistry))
	for name := range defaultRegistry {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func HasBuiltinMapping(name string) bool {
	_, ok := defaultRegistry[CanonicalName(name)]
	return ok
}

func NormalizeRegistry(custom map[string]string) map[string]string {
	result := DefaultRegistry()
	for rawName, rawID := range custom {
		name := CanonicalName(rawName)
		value := strings.TrimSpace(rawID)
		if name == "" || value == "" {
			continue
		}
		result[name] = value
	}
	return result
}

func NormalizeNameValueMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for rawName, rawID := range values {
		name := CanonicalName(rawName)
		value := strings.TrimSpace(rawID)
		if name == "" || value == "" {
			continue
		}
		result[name] = value
	}
	return result
}

func OfficialCatalog() []OfficialModel {
	result := make([]OfficialModel, 0, len(officialCatalog))
	for _, item := range officialCatalog {
		next := item
		next.BuiltinMapping = HasBuiltinMapping(item.Name)
		result = append(result, next)
	}
	return result
}
