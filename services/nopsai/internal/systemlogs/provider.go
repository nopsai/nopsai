package systemlogs

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSourceNotFound     = errors.New("system log source not found")
	ErrCursorInvalid      = errors.New("system log cursor is invalid")
	ErrCursorExpired      = errors.New("system log cursor has expired")
	ErrStreamLimit        = errors.New("system log stream limit reached")
	ErrReconnectRateLimit = errors.New("system log reconnect rate limit reached")
)

type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
)

type Entry struct {
	ID                string    `json:"id"`
	SourceID          string    `json:"source_id"`
	ContainerName     string    `json:"container_name"`
	ContainerInstance string    `json:"container_instance"`
	EmittedAt         time.Time `json:"emitted_at"`
	ObservedAt        time.Time `json:"observed_at"`
	Stream            Stream    `json:"stream"`
	Line              string    `json:"line"`
	sequence          uint64
}

type SourceStatus struct {
	ID                string `json:"id"`
	DisplayName       string `json:"display_name"`
	ContainerName     string `json:"container_name"`
	ContainerInstance string `json:"container_instance,omitempty"`
	Available         bool   `json:"available"`
	State             string `json:"state"`
	Health            string `json:"health,omitempty"`
	Status            string `json:"status,omitempty"`
}

type Cursor struct {
	SourceID          string    `json:"source_id"`
	ContainerInstance string    `json:"container_instance"`
	Sequence          uint64    `json:"sequence"`
	EmittedAt         time.Time `json:"emitted_at"`
}

type Provider interface {
	ListSources(ctx context.Context) ([]SourceStatus, error)
	Tail(ctx context.Context, sourceID string, lines int) ([]Entry, error)
	Follow(ctx context.Context, sourceID string, after Cursor, emit func(Entry)) error
}
