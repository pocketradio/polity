package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// completes the mcp handshake before tool calls begin
func (c *Client) Initialize(ctx context.Context) error {
	_, err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "polity-agent",
			"version": "0.1.0",
		},
	})
	if err != nil {
		// return the handshake failure to the agent
		return err
	}
	return c.notify("notifications/initialized", map[string]any{})
}

// invokes one artifice tool and returns its text content
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	result, err := c.request(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		// return transport failures before decoding tool content
		return "", err
	}

	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		// return malformed tool responses with the tool name
		return "", fmt.Errorf("decode %s result: %w", name, err)
	}
	var text []string
	for _, item := range payload.Content {
		if item.Type == "text" {
			// keep only text content for the agent loop
			text = append(text, item.Text)
		}
	}
	joined := strings.Join(text, "\n")
	if payload.IsError {
		// convert mcp tool errors into the agent error path
		return joined, fmt.Errorf("%s returned an MCP error: %s", name, joined)
	}
	return joined, nil
}
