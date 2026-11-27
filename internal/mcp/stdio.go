package mcp

import (
	"bufio"
	"context"
	"io"
	"sync"
)

// ServeStdio speaks MCP over newline-delimited JSON on in/out for a local
// process, until in ends or ctx is cancelled. Writes follow mcp.scope.
func (s *Service) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sess := s.NewSession(KeyLocal)
	defer s.CloseSession(sess.ID)
	w := &lineWriter{w: out}
	done := make(chan struct{})
	go func() { defer close(done); forward(ctx, sess, w) }()
	err := s.readLines(ctx, sess, in, w)
	cancel()
	<-done
	return err
}

func (s *Service) readLines(ctx context.Context, sess *Session, in io.Reader, w *lineWriter) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), maxBody)
	for sc.Scan() && ctx.Err() == nil {
		if len(sc.Bytes()) == 0 {
			continue
		}
		if out := s.Handle(ctx, sess, append([]byte{}, sc.Bytes()...)); out != nil {
			if err := w.line(out); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// forward writes the session's notifications between responses.
func forward(ctx context.Context, sess *Session, w *lineWriter) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sess.out:
			_ = w.line(msg)
		}
	}
}

// lineWriter keeps responses and notifications whole on one stream.
type lineWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lineWriter) line(b []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.w.Write(append(b, '\n'))
	return err
}
