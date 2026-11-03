package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// sends one request and waits for the response with the matching id
func (c *Client) request(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()

	// mutex to prevent concurrent reqs. each client will have its own agent assigned
	// so this lock is only to prevent interleaving of requests and responses on the same client
	defer c.mu.Unlock()

	id := c.nextID
	c.nextID++
	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	encoded, err := json.Marshal(request)
	if err != nil {
		// return request encoding failures
		return nil, err
	}
	if _, err := fmt.Fprintf(c.stdin, "%s\n", encoded); err != nil {
		// return failures writing to the mcp process
		return nil, err
	}

	// scan notifications and responses until this request id is found
	for c.stdout.Scan() {
		select {
		case <-ctx.Done():
			// stop waiting when the caller cancels the request
			return nil, ctx.Err()
		default:
		}
		var response rpcResponse
		if err := json.Unmarshal(c.stdout.Bytes(), &response); err != nil {
			// ignore non-json child output and continue scanning
			continue
		}
		if response.ID != id {
			// ignore responses belonging to another request
			continue
		}
		if response.Error != nil {
			// return the server error for the matching request
			return nil, fmt.Errorf("MCP %d: %s", response.Error.Code, response.Error.Message)
		}
		return response.Result, nil
	}
	if err := c.stdout.Err(); err != nil {
		// return scanner failures after the child process closes
		return nil, err
	}
	return nil, io.EOF
}

// sends a json rpc notification that does not require a response
func (c *Client) notify(method string, params map[string]any) error {
	request := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	encoded, err := json.Marshal(request)
	if err != nil {
		// return notification encoding failures
		return err
	}
	_, err = fmt.Fprintf(c.stdin, "%s\n", encoded)
	return err
}
