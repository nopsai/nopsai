package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type StepSession struct {
	Name string
	ID   string
}

type StepSessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]string
}

func NewStepSessionRegistry() *StepSessionRegistry {
	return &StepSessionRegistry{sessions: map[string]string{}}
}

func (r *StepSessionRegistry) Get(name string) (string, bool) {
	if r == nil {
		return "", false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.sessions[name]
	return id, ok
}

func (r *StepSessionRegistry) Set(name, id string) {
	if r == nil {
		return
	}
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	if name == "" || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[name] = id
}

func (r *StepSessionRegistry) GetOrCreate(name string, create func() (string, error)) (string, bool, error) {
	if r == nil {
		return "", false, fmt.Errorf("step session registry is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, fmt.Errorf("step session name is required")
	}
	if create == nil {
		return "", false, fmt.Errorf("step session create function is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if id, ok := r.sessions[name]; ok {
		return id, false, nil
	}
	id, err := create()
	if err != nil {
		return "", false, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false, fmt.Errorf("step session create function returned an empty id")
	}
	r.sessions[name] = id
	return id, true, nil
}

func (r *StepSessionRegistry) Clear() []StepSession {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	sessions := make([]StepSession, 0, len(r.sessions))
	for name, id := range r.sessions {
		sessions = append(sessions, StepSession{Name: name, ID: id})
	}
	r.sessions = map[string]string{}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})
	return sessions
}
