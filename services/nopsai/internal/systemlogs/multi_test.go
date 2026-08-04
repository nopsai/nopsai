package systemlogs

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testProvider struct {
	sources []SourceStatus
	tail    []Entry
	err     error
}

func (p testProvider) ListSources(context.Context) ([]SourceStatus, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.sources, nil
}

func (p testProvider) Tail(context.Context, string, int) ([]Entry, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.tail == nil {
		return nil, ErrSourceNotFound
	}
	return p.tail, nil
}

func (p testProvider) Follow(_ context.Context, _ string, _ Cursor, emit func(Entry)) error {
	if p.err != nil {
		return p.err
	}
	if p.tail == nil {
		return ErrSourceNotFound
	}
	for _, entry := range p.tail {
		emit(entry)
	}
	return nil
}

func TestMultiProviderMergesSourcesPreferAvailable(t *testing.T) {
	provider := NewMultiProvider(
		testProvider{sources: []SourceStatus{{ID: "dispatcher", State: "unavailable"}}},
		testProvider{sources: []SourceStatus{{ID: "dispatcher", Available: true, State: "running"}, {ID: "runner:prod", Available: true}}},
	)
	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 2 || sources[0].ID != "dispatcher" || !sources[0].Available || sources[1].ID != "runner:prod" {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestMultiProviderFallsBackForTailAndFollow(t *testing.T) {
	want := []Entry{{SourceID: "runner:prod", Line: "ready", EmittedAt: time.Unix(10, 0)}}
	provider := NewMultiProvider(
		testProvider{err: ErrSourceNotFound},
		testProvider{tail: want},
	)
	entries, err := provider.Tail(context.Background(), "runner:prod", 1)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Line != "ready" {
		t.Fatalf("entries = %#v", entries)
	}
	var followed []Entry
	if err := provider.Follow(context.Background(), "runner:prod", Cursor{}, func(entry Entry) { followed = append(followed, entry) }); err != nil {
		t.Fatalf("Follow() error = %v", err)
	}
	if len(followed) != 1 || followed[0].Line != "ready" {
		t.Fatalf("followed = %#v", followed)
	}
}

func TestMultiProviderReturnsFirstOperationalError(t *testing.T) {
	boom := errors.New("provider unavailable")
	provider := NewMultiProvider(testProvider{err: boom}, testProvider{err: ErrSourceNotFound})
	if _, err := provider.Tail(context.Background(), "dispatcher", 1); !errors.Is(err, boom) {
		t.Fatalf("Tail() error = %v, want first operational error", err)
	}
}
