package theater

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/alex-poliushkin/theater/observe"
)

type boundaryPanicError struct {
	family    string
	ref       string
	recovered any
	stack     []byte
}

type containedObserverError struct {
	event   Event
	failure *Failure
	cause   error
}

type containedDebugBoundaryError struct {
	failure *Failure
	cause   error
}

type containedStageFailure interface {
	error
	stageFailure() *Failure
}

type panicCapturingSink struct {
	mu   sync.Mutex
	sink observe.Sink
	ref  string
	err  error
}

func invokeBoundary[T any](family, ref string, fn func() (T, error)) (value T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = boundaryPanicError{
				family:    family,
				ref:       ref,
				recovered: recovered,
				stack:     debug.Stack(),
			}
		}
	}()

	return fn()
}

func invokeBoundaryError(family, ref string, fn func() error) error {
	_, err := invokeBoundary(family, ref, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

func newContainedObserverError(event Event, summary string, cause error) error {
	path := event.Path
	if path == "" {
		path = event.StagePath
	}

	return containedObserverError{
		event:   event,
		failure: internalFailure(path, summary, cause),
		cause:   cause,
	}
}

func newContainedDebugBoundaryError(state debugBoundaryState, summary string, cause error) error {
	path := state.Ref.Path
	if path == "" {
		path = state.Ref.StagePath
	}

	return containedDebugBoundaryError{
		failure: internalFailure(path, summary, cause),
		cause:   cause,
	}
}

func newPanicCapturingSink(sink observe.Sink, ref string) *panicCapturingSink {
	if sink == nil {
		return nil
	}

	return &panicCapturingSink{
		sink: sink,
		ref:  ref,
	}
}

func (e boundaryPanicError) Error() string {
	if e.ref == "" {
		return fmt.Sprintf("%s panicked: %v", e.family, e.recovered)
	}

	return fmt.Sprintf("%s %q panicked: %v", e.family, e.ref, e.recovered)
}

func (e boundaryPanicError) Stack() []byte {
	return append([]byte(nil), e.stack...)
}

func (e containedObserverError) Error() string {
	return e.failure.Message()
}

func (e containedObserverError) Unwrap() error {
	return e.cause
}

func (e containedObserverError) stageFailure() *Failure {
	return e.failure
}

func (e containedDebugBoundaryError) Error() string {
	return e.failure.Message()
}

func (e containedDebugBoundaryError) Unwrap() error {
	return e.cause
}

func (e containedDebugBoundaryError) stageFailure() *Failure {
	return e.failure
}

func (s *panicCapturingSink) Publish(env observe.Envelope) uint64 {
	if s == nil {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sink == nil || s.err != nil {
		return 0
	}

	dropped, err := invokeBoundary("live sink", s.ref, func() (uint64, error) {
		return s.sink.Publish(env), nil
	})
	if err != nil {
		s.err = err
		return 0
	}

	return dropped
}

func (s *panicCapturingSink) Failure() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.err
}
