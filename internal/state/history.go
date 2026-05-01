package state

import (
	"encoding/json"
	"os"
	"time"
)

// HistoryEvent is one line in history.jsonl.
type HistoryEvent struct {
	Timestamp time.Time         `json:"ts"`
	Op        string            `json:"op"` // install | upgrade | uninstall | port | adopt
	Skill     string            `json:"skill"`
	Version   string            `json:"version,omitempty"`
	From      string            `json:"from,omitempty"`
	To        string            `json:"to,omitempty"`
	Targets   []string          `json:"targets,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// AppendHistory appends an event to history.jsonl. Best-effort; an error is
// returned but callers typically log-and-continue rather than fail the
// containing operation.
func AppendHistory(ev HistoryEvent) error {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	path, err := historyPath()
	if err != nil {
		return err
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
