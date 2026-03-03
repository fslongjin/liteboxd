package cluster

import (
	"sort"
	"strings"
)

func canonicalizeSettings(settings []string) []string {
	out := make([]string, 0, len(settings))
	for _, setting := range settings {
		normalized := strings.TrimSpace(setting)
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}
