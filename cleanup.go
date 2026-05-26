package main

import (
	"errors"
	"fmt"
	"sync"
)

type cleanupAction struct {
	name string
	fn   func() error
}

type CleanupStack struct {
	mu      sync.Mutex
	actions []cleanupAction
	ran     bool
	done    chan struct{}
	result  error
}

func NewCleanupStack() *CleanupStack {
	return &CleanupStack{}
}

func (s *CleanupStack) Add(name string, fn func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ran {
		return
	}

	s.actions = append(s.actions, cleanupAction{name: name, fn: fn})
}

func (s *CleanupStack) Run() error {
	s.mu.Lock()
	if s.ran {
		done := s.done
		s.mu.Unlock()
		<-done

		s.mu.Lock()
		defer s.mu.Unlock()
		return s.result
	}
	actions := append([]cleanupAction(nil), s.actions...)
	s.ran = true
	s.done = make(chan struct{})
	done := s.done
	s.mu.Unlock()

	errs := make([]error, 0)
	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if err := action.fn(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", action.name, err))
		}
	}

	result := errors.Join(errs...)

	s.mu.Lock()
	s.result = result
	close(done)
	s.mu.Unlock()

	return result
}
