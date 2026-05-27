package dns

import (
	"errors"
	"fmt"
	"sync"
)

type cleanupAction struct {
	name string
	fn   func() error
}

type cleanupStack struct {
	mu      sync.Mutex
	actions []cleanupAction
}

func newCleanupStack() *cleanupStack {
	return &cleanupStack{}
}

func (s *cleanupStack) Add(name string, fn func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, cleanupAction{name: name, fn: fn})
}

func (s *cleanupStack) Run() error {
	s.mu.Lock()
	actions := append([]cleanupAction(nil), s.actions...)
	s.actions = nil
	s.mu.Unlock()

	errs := make([]error, 0)
	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if err := action.fn(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", action.name, err))
		}
	}
	return errors.Join(errs...)
}
