package mcp

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type Client struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Scanner
	nextID  int
	mu      sync.Mutex
}

// starts the configured mcp server and connects its standard streams
func Start(ctx context.Context) (*Client, error) {
	bin := os.Getenv("POLITY_MCP_BIN")
	if bin == "" {
		// use node when no mcp executable is configured
		bin = "node"
	}
	args := splitArgs(os.Getenv("POLITY_MCP_ARGS"))
	if len(args) == 0 {
		// use the checked out artifice mcp client by default
		args = []string{filepath.Join("..", "artifice", "apps", "mcp-client", "dist", "index.js")}
	}

	command := exec.CommandContext(ctx, bin, args...)
	command.Stderr = os.Stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		// return when the child stdin cannot be connected
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		// return when the child stdout cannot be connected
		return nil, err
	}
	if err := command.Start(); err != nil {
		// return when the mcp child process cannot start
		return nil, err
	}
	return &Client{command: command, stdin: stdin, stdout: bufio.NewScanner(stdout), nextID: 1}, nil
}

// closes the child process streams and waits for process exit
func (c *Client) Close() error {
	_ = c.stdin.Close()
	return c.command.Wait()
}
