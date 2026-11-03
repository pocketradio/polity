package recording

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Decision struct {
	Timestamp time.Time      `json:"timestamp"`
	Step      string         `json:"step"`
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	LatencyMS int64          `json:"latency_ms"`
	Error     string         `json:"error,omitempty"`
}

type Recorder struct {
	file *os.File
	out  *bufio.Writer
}

// opens an append-only json lines decision log
func New(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		// return when the log directory cannot be created
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// return when the log file cannot be opened
		return nil, err
	}
	return &Recorder{file: file, out: bufio.NewWriter(file)}, nil
}

// writes one decision record and flushes it immediately
func (r *Recorder) Write(decision Decision) error {
	data, err := json.Marshal(decision)
	if err != nil {
		// return when a decision cannot be encoded
		return err
	}
	if _, err := r.out.Write(append(data, '\n')); err != nil {
		// return when the encoded decision cannot be written
		return err
	}
	return r.out.Flush()
}

// flushes buffered output and closes the log file
func (r *Recorder) Close() error {
	if err := r.out.Flush(); err != nil {
		// close the file even when buffered output fails
		_ = r.file.Close()
		return err
	}
	return r.file.Close()
}
