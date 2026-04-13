package common

import (
	"os"
	"strings"

	"github.com/iocgo/sdk/env"
)

// IsEnvPlaceholder reports whether a config value still looks like an
// unresolved bootstrap placeholder such as ${PASSWORD}.
func IsEnvPlaceholder(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 4 && strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}")
}

// ResolveEnvFallback keeps explicit config values, but falls back to an
// environment variable when the config is empty or still contains an
// unresolved ${VAR} placeholder.
func ResolveEnvFallback(value string, fallbackEnv string) string {
	value = strings.TrimSpace(value)
	if value != "" && !IsEnvPlaceholder(value) {
		return value
	}
	if fallbackEnv == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(fallbackEnv))
}

func ResolveServerPassword(environment *env.Environment) string {
	if environment == nil {
		return ResolveEnvFallback("", "PASSWORD")
	}
	return ResolveEnvFallback(environment.GetString("server.password"), "PASSWORD")
}
