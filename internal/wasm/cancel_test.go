package wasm

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestCallerCancelIsNotAFailure(t *testing.T) {
	h := loadHostile(t)
	p, _ := h.m.NewPool(Binding{Key: "c", Settings: map[string]any{"attack": "loop"}})
	defer p.Drain(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	time.AfterFunc(100*time.Millisecond, cancel)
	if err := NewHandle(p).Test(ctx); plugin.ClassOf(err) != plugin.ClassTimeout {
		t.Errorf("cancelled call: %v", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failures != 0 {
		t.Errorf("cancellation counted as a failure: %d", p.failures)
	}
}
