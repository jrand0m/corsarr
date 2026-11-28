package conformance

import (
	"fmt"
	"time"
)

// Check is one named result of a suite run outside go test.
type Check struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Problems []string      `json:"problems,omitempty"`
	Took     time.Duration `json:"took"`
}

// recorder implements T by collecting failures; Fatal stops the check.
type recorder struct {
	problems []string
	cleanups []func()
}

type fatal struct{}

func (r *recorder) Helper()                   {}
func (r *recorder) Error(args ...any)         { r.problems = append(r.problems, fmt.Sprint(args...)) }
func (r *recorder) Errorf(f string, a ...any) { r.problems = append(r.problems, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatal(args ...any)         { r.Error(args...); panic(fatal{}) }
func (r *recorder) Fatalf(f string, a ...any) { r.Errorf(f, a...); panic(fatal{}) }
func (r *recorder) Cleanup(fn func())         { r.cleanups = append(r.cleanups, fn) }

// Run runs one check with a recording T: failures are collected, Fatal ends
// the check, and a panic in the plugin is reported as a failure.
func Run(name string, fn func(t T)) (c Check) {
	r := &recorder{}
	start := time.Now()
	defer func() {
		if p := recover(); p != nil {
			if _, ok := p.(fatal); !ok {
				r.problems = append(r.problems, fmt.Sprintf("panic: %v", p))
			}
		}
		for i := len(r.cleanups) - 1; i >= 0; i-- {
			r.cleanups[i]()
		}
		c = Check{Name: name, Passed: len(r.problems) == 0, Problems: r.problems, Took: time.Since(start)}
	}()
	fn(r)
	return Check{}
}
