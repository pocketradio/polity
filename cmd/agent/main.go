package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketradio/polity/internal/decision"
	"github.com/pocketradio/polity/internal/mcp"
	"github.com/pocketradio/polity/internal/recording"
)

// runs one baseline agent from startup through match completion
func main() {
	ctx := context.Background()

	// create the append-only decision log before starting the game
	logPath := getenv("POLITY_LOG_PATH", filepath.Join("data", "decisions.jsonl"))
	recorder, err := recording.New(logPath)
	// stop when the decision log cannot be opened
	if err != nil {
		fatal(err)
	}
	defer recorder.Close()

	// launch and initialize the local artifice mcp server
	client, err := mcp.Start(ctx)
	// stop when the mcp server cannot start
	if err != nil {
		fatal(err)
	}
	defer client.Close()

	// stop when the mcp handshake fails
	if err := client.Initialize(ctx); err != nil {
		fatal(err)
	}

	// read the rules and enter a development match before the turn loop
	strategy := getenv("POLITY_STRATEGY", "Play conservatively. Preserve troops and avoid unnecessary combat.")
	engine := decision.FromEnv()
	// stop when the rules request fails
	if _, err := callAndRecord(ctx, client, recorder, "rules", nil, "rules"); err != nil {
		fatal(err)
	}
	// stop when matchmaking fails
	if _, err := callAndRecord(ctx, client, recorder, "find_game", map[string]any{
		"strategy": strategy,
		"mode":     getenv("POLITY_GAME_MODE", "dev"),
	}, "queue"); err != nil {
		fatal(err)
	}

	// observe the game and submit the safe fallback action once per turn
	maxTurns := 30
	// continue until artifice reports completion or the safety limit is reached
	for turn := 1; turn <= maxTurns; turn++ {
		state, err := callAndRecord(ctx, client, recorder, "get_game_state", nil, fmt.Sprintf("observe-%d", turn))
		// stop when state retrieval fails
		if err != nil {
			fatal(err)
		}
		// exit after the terminal match state is recorded
		if finished(state) {
			return
		}

		// choose actions from the configured llm or use the empty fallback
		decisionStarted := time.Now()
		actions, rawDecision, decisionErr := engine.Choose(ctx, strategy, state)
		// fall back to no actions when the llm request or parsing fails
		if decisionErr != nil {
			actions = []any{}
		}
		_ = recorder.Write(recording.Decision{
			Timestamp: time.Now().UTC(),
			Step:      fmt.Sprintf("decide-%d", turn),
			Tool:      "llm_decision",
			Input:     map[string]any{"strategy": strategy, "state": state},
			Output:    rawDecision,
			LatencyMS: time.Since(decisionStarted).Milliseconds(),
			Error:     errorText(decisionErr),
		})

		// submit the selected action batch through artifice
		if _, err := callAndRecord(ctx, client, recorder, "submit_actions", map[string]any{
			"actions": actions,
		}, fmt.Sprintf("submit-%d", turn)); err != nil {
			// stop when action submission fails
			fatal(err)
		}
	}

	fatal(fmt.Errorf("match did not finish after %d turns", maxTurns))
}

// calls one mcp tool and records its complete result
func callAndRecord(ctx context.Context, client *mcp.Client, recorder *recording.Recorder, tool string, args map[string]any, step string) (string, error) {
	started := time.Now()
	result, err := client.CallTool(ctx, tool, args)
	recordErr := recorder.Write(recording.Decision{
		Timestamp: time.Now().UTC(),
		Step:      step,
		Tool:      tool,
		Input:     args,
		Output:    result,
		LatencyMS: time.Since(started).Milliseconds(),
		Error:     errorText(err),
	})
	if err != nil {
		// return the mcp failure after recording it
		return "", err
	}
	if recordErr != nil {
		// return the logging failure before continuing the match
		return "", recordErr
	}
	return result, nil
}

// identifies the terminal state markers returned by artifice
func finished(state string) bool {
	text := strings.ToLower(state)
	return strings.Contains(text, "game over") || strings.Contains(text, "phase: finished") || strings.Contains(text, "phase: game_over")
}

// converts an optional err into the value stored in the decision log
func errorText(err error) string {
	if err == nil {
		// represent a successful call with an empty error field
		return ""
	}
	return err.Error()
}

// returns an environment value or its local default
func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		// use the configured environment value
		return value
	}
	return fallback
}

// writes a fatal startup or match error and exits the process
func fatal(err error) {
	fmt.Fprintln(os.Stderr, "polity-agent:", err)
	os.Exit(1)
}
