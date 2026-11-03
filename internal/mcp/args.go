package mcp

import "strings"

// splits an optional command argument string for the child process
func splitArgs(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Fields(value)
}
