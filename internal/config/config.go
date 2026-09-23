package config

import (
	"os"
	"strings"
)

// DefaultHost resolves the configured GitHub host the same way as gh.
func DefaultHost() string {
	for _, key := range []string{"GH_HOST", "GH_ENTERPRISE_HOST"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return normalizeHost(value)
		}
	}
	return "github.com"
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	if index := strings.Index(host, "/"); index >= 0 {
		host = host[:index]
	}
	if host == "" {
		return "github.com"
	}
	return host
}
