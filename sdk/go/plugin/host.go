package plugin

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// call sends a request to a host function and decodes its result.
func call(name string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	out, err := rawHost(name, body)
	if err != nil {
		return err
	}
	var env abi.Envelope
	if err := json.Unmarshal(out, &env); err != nil {
		return Errorf(abi.ClassParse, "host answered malformed JSON: %v", err)
	}
	if env.Error != nil {
		return env.Error
	}
	if resp != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, resp)
	}
	return nil
}

// Log writes a line to corsarr's log, tagged with the plugin and instance.
// attrs are key, value pairs.
func Log(level, msg string, attrs ...any) {
	m := map[string]any{}
	for i := 0; i+1 < len(attrs); i += 2 {
		m[fmt.Sprint(attrs[i])] = attrs[i+1]
	}
	_ = call(abi.HostLog, abi.LogRequest{Level: level, Message: msg, Attrs: m}, nil)
}

// Debug logs at debug level.
func Debug(msg string, attrs ...any) { Log("debug", msg, attrs...) }

// Info logs at info level.
func Info(msg string, attrs ...any) { Log("info", msg, attrs...) }

// Warn logs at warn level.
func Warn(msg string, attrs ...any) { Log("warn", msg, attrs...) }

// Settings returns the instance's non-secret settings (defaults filled in;
// durations as text such as "30s", maps as objects).
func Settings() (map[string]any, error) {
	var r abi.SettingsResponse
	if err := call(abi.HostSettingsGet, Empty{}, &r); err != nil {
		return nil, err
	}
	return r.Values, nil
}

// SettingString returns one text setting, or "".
func SettingString(key string) string {
	s, err := Settings()
	if err != nil {
		return ""
	}
	v, _ := s[key].(string)
	return v
}

// Secret reads a secret setting declared in permissions.secrets.
func Secret(name string) (string, error) {
	var r abi.SecretResponse
	err := call(abi.HostSecretGet, abi.SecretRequest{Name: name}, &r)
	return r.Value, err
}

// Now is the host clock.
func Now() time.Time {
	var r abi.NowResponse
	if err := call(abi.HostNow, Empty{}, &r); err != nil {
		return time.Now()
	}
	return time.Unix(0, r.UnixNano)
}

// Sleep pauses the call; the host stops it at the call deadline.
func Sleep(d time.Duration) error {
	return call(abi.HostSleep, abi.SleepRequest{Millis: d.Milliseconds()}, nil)
}

// Emit publishes an event; only the event types allowed for the module's kind
// are accepted.
func Emit(eventType string, data map[string]any) error {
	return call(abi.HostEmitEvent, abi.EventRequest{Type: eventType, Data: data}, nil)
}
