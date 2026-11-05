package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Engine struct {
	URL   string
	Key   string
	Model string
}

type response struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// builds an llm engine from environment configuration
func FromEnv() *Engine {
	return &Engine{
		URL:   os.Getenv("POLITY_LLM_URL"),
		Key:   os.Getenv("POLITY_LLM_API_KEY"),
		Model: os.Getenv("POLITY_LLM_MODEL"),
	}
}

// chooses a structured action batch from the current game state
func (e *Engine) Choose(ctx context.Context, strategy, state string) ([]any, string, error) {
	if e.URL == "" || e.Model == "" {
		// use the safe fallback when llm configuration is absent
		return []any{}, "", nil
	}

	prompt := "Return only a JSON array of legal Artifice actions. Use an empty array when no safe action is clear.\n" +
		"Strategy: " + strategy + "\nCurrent game state:\n" + state
	body := map[string]any{
		"model": e.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a careful strategy game agent. Output only a JSON action array."},
			{"role": "user", "content": prompt},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		// return request encoding failures
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL, bytes.NewReader(encoded))
	if err != nil {
		// return request construction failures
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.Key != "" {
		req.Header.Set("Authorization", "Bearer "+e.Key)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		// return network failures to the fallback path
		return nil, "", err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		// return response read failures
		return nil, "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// return provider errors with the raw response
		return nil, string(data), fmt.Errorf("llm request failed with status %s", res.Status)
	}

	var parsed response
	if err := json.Unmarshal(data, &parsed); err != nil {
		// return malformed provider responses
		return nil, string(data), err
	}
	if len(parsed.Choices) == 0 {
		// return responses without a model choice
		return nil, string(data), fmt.Errorf("llm response contained no choices")
	}
	raw := parsed.Choices[0].Message.Content
	return parseActions(raw)
}

// parses the model response into the action batch accepted by artifice
func parseActions(raw string) ([]any, string, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var actions []any
	if err := json.Unmarshal([]byte(clean), &actions); err != nil {
		// reject output that is not a json action array
		return nil, raw, fmt.Errorf("decode llm action array: %w", err)
	}
	return actions, raw, nil
}
