package config

import (
	"os"
	"strings"
)

// EmbeddedEnv holds the embedded .env content.
// This will be set at build time via -ldflags.
var EmbeddedEnv string

// LoadEmbedded parses the embedded env content and sets environment variables.
// Variables are only set if not already defined (existing env takes precedence).
func LoadEmbedded() {
	if EmbeddedEnv == "" {
		return
	}

	content := strings.ReplaceAll(EmbeddedEnv, `\n`, "\n")

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Only set if not already defined
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
