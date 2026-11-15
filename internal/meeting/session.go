package meeting

import (
	"bufio"
	"context"
	"errors"
	"io"
	"time"
)

// TickInterval is how often a running session checks for pauses, due
// heartbeats and expired offers. It bounds how late a pause is noticed.
const TickInterval = 100 * time.Millisecond

// Session connects an Engine to an encoder and a Decider. Run drives it
// from real stdin and the wall clock; tests drive it step by step with a
// fake clock, with decisions taken synchronously so the replay is exact.
type Session struct {
	Engine  *Engine
	Encoder *Encoder
	Decider Decider
	// Settings supplies the decision and summary timeouts.
	Settings Settings
	// Async runs each decision in its own goroutine, delivering the result
	// on Results; Run sets it so a slow model never stalls stdin or
	// heartbeats. Tests leave it false.
	Async   bool
	Results chan Decided

	inFlight *Decided
}

// Decided is a finished decision waiting to be applied.
type Decided struct {
	Req  Request
	Resp Response
	Err  error
}

func (s *Session) emit(in []Intent) error {
	for _, i := range in {
		if err := s.Encoder.Emit(i); err != nil {
			return err
		}
	}
	return nil
}

// Start emits the opening heartbeat.
func (s *Session) Start(now time.Time) error { return s.emit(s.Engine.Start(now)) }

// Line handles one stdin line and starts a decision if one is due.
func (s *Session) Line(b []byte, now time.Time) error {
	if err := s.emit(s.Engine.HandleLine(b, now)); err != nil {
		return err
	}
	return s.maybeDecide(now)
}

// Tick handles the passage of time.
func (s *Session) Tick(now time.Time) error {
	if err := s.emit(s.Engine.Tick(now)); err != nil {
		return err
	}
	return s.maybeDecide(now)
}

// Deliver applies a finished asynchronous decision.
func (s *Session) Deliver(d Decided, now time.Time) error {
	s.inFlight = nil
	if err := s.emit(s.Engine.Apply(d.Req, d.Resp, d.Err, now)); err != nil {
		return err
	}
	return s.maybeDecide(now)
}

// Pending reports whether an asynchronous decision is outstanding.
func (s *Session) Pending() bool { return s.inFlight != nil }

func (s *Session) maybeDecide(now time.Time) error {
	req, ok := s.Engine.Due(now)
	if !ok {
		return nil
	}
	if !s.Async {
		resp, err := s.call(req, s.Settings.DecisionTimeout)
		return s.emit(s.Engine.Apply(req, resp, err, now))
	}
	s.inFlight = &Decided{Req: req}
	go func() {
		resp, err := s.call(req, s.Settings.DecisionTimeout)
		s.Results <- Decided{Req: req, Resp: resp, Err: err}
	}()
	return nil
}

func (s *Session) call(req Request, timeout time.Duration) (resp Response, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("decider panicked")
		}
	}()
	return s.Decider.Decide(ctx, req)
}

// Close takes the closing summary decision and emits the summary.
func (s *Session) Close(now time.Time) error {
	resp, err := s.call(s.Engine.SummaryRequest(now), s.Settings.SummaryTimeout)
	return s.emit(s.Engine.Finish(resp, err))
}

// Run reads events from in until an end event, EOF or ctx is done, writing
// intents through the session's encoder. It returns nil on a normal end, so
// the process exits 0; only a failure to write stdout is an error.
func Run(ctx context.Context, s *Session, in io.Reader, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	s.Async = true
	s.Results = make(chan Decided, 1)

	lines := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		r := bufio.NewReaderSize(in, readBufferBytes)
		for {
			b, err := r.ReadBytes('\n')
			if len(b) > 0 {
				lines <- b
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				readErr <- err
				return
			}
		}
	}()

	if err := s.Start(now()); err != nil {
		return err
	}
	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			s.Engine.logf("stopping: %v", ctx.Err())
			break loop
		case b := <-lines:
			if err := s.Line(b, now()); err != nil {
				return err
			}
			if s.Engine.Ended() {
				break loop
			}
		case err := <-readErr:
			if err != nil {
				s.Engine.logf("reading stdin: %v", err)
			}
			break loop
		case d := <-s.Results:
			if err := s.Deliver(d, now()); err != nil {
				return err
			}
		case <-ticker.C:
			if err := s.Tick(now()); err != nil {
				return err
			}
		}
	}

	// Let a decision already under way land, so its notes reach the
	// summary; it is bounded by its own timeout.
	if s.Pending() {
		select {
		case d := <-s.Results:
			if err := s.Deliver(d, now()); err != nil {
				return err
			}
		case <-time.After(s.Settings.DecisionTimeout + TickInterval):
		}
	}
	return s.Close(now())
}

// readBufferBytes sizes the stdin reader; a longer line is still read
// whole, this only sets the initial buffer.
const readBufferBytes = 64 * 1024
