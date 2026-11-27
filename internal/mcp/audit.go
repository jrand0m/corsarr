package mcp

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Call outcomes recorded in History.
const (
	outcomeOK      = "ok"
	outcomeError   = "error"
	outcomeRefused = "refused"
	outcomeLimited = "rate_limited"
)

// CallData is the data of mcp.called.
type CallData struct {
	Client    string         `json:"client"`
	Key       Key            `json:"key"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Outcome   string         `json:"outcome"`
	Error     string         `json:"error,omitempty"`
}

func outcomeOf(err error) string {
	var me domain.ModeError
	switch {
	case err == nil:
		return outcomeOK
	case errors.As(err, &me):
		return outcomeRefused
	}
	return outcomeError
}

// audit records one tool call in History.
func (s *Service) audit(sess *Session, p callParams, outcome string, err error) {
	data := CallData{Client: sess.Client(), Key: sess.key, Tool: p.Name, Arguments: redact(p.Arguments), Outcome: outcome}
	if err != nil {
		data.Error = errText(err)
	}
	_, perr := s.d.Bus.Publish(domain.Event{Type: domain.EvMCPCalled, Actor: "mcp:" + data.Client, Data: data})
	if perr != nil {
		s.d.Log.Warn("mcp audit", "err", perr)
	}
}

// secretWords mark argument names whose values never reach History.
var secretWords = []string{"key", "token", "password", "secret", "cookie", "auth"}

// redact copies the arguments with secret-looking values replaced.
func redact(raw json.RawMessage) map[string]any {
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil {
		return nil
	}
	for k := range args {
		if secretName(k) {
			args[k] = "[redacted]"
		}
	}
	return args
}

func secretName(name string) bool {
	lower := strings.ToLower(name)
	for _, w := range secretWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
