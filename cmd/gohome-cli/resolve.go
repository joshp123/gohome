package main

import (
	"fmt"
	"sort"
	"strings"
)

func normalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer(" ", "_", "-", "_", "__", "_")
	name = replacer.Replace(name)
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	return name
}

func resolveNamedID(kind, input string, options map[string]string) (string, error) {
	needle := normalizeName(input)
	for label, id := range options {
		if normalizeName(label) == needle {
			return id, nil
		}
	}
	available := make([]string, 0, len(options))
	for label := range options {
		available = append(available, label)
	}
	sort.Strings(available)
	return "", fmt.Errorf("%s %q not found. Available: %s", kind, input, strings.Join(available, ", "))
}
