package systemlogs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	mu      sync.Mutex
	tail    []Entry
	follows []chan Entry
	tailErr error
}

func (f *fakeProvider) ListSources(context.Context) ([]SourceStatus, error) {
	return []SourceStatus{{ID: "dispatcher", Available: true, State: "running"}}, nil
}
func (f *fakeProvider) Tail(context.Context, string, int) ([]Entry, error) {
	return append([]Entry(nil), f.tail...), f.tailErr
}
func (f *fakeProvider) Follow(ctx context.Context, _ string, _ Cursor, emit func(Entry)) error {
	ch := make(chan Entry, 4)
	f.mu.Lock()
	f.follows = append(f.follows, ch)
	f.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entry := <-ch:
			emit(entry)
		}
	}
}
func (f *fakeProvider) emit(t *testing.T, entry Entry) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		f.mu.Lock()
		if len(f.follows) > 0 {
			ch := f.follows[len(f.follows)-1]
			f.mu.Unlock()
			ch <- entry
			return
		}
		f.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("collector did not start")
		}
		time.Sleep(time.Millisecond)
	}
}

func newTestBroker(provider Provider, capacity, maxStreams int) *Broker {
	return NewBroker(provider, NewRegistry([]Source{{ID: "dispatcher", ContainerName: "nopsai-dispatcher"}}), NewRedactor(1024), NewCursorCodec([]byte("test")), BrokerOptions{
		BufferLines: capacity, BufferAge: time.Hour, MaxTailLines: 50, MaxStreams: maxStreams, MaxStreamsPerSource: maxStreams, RetryDelay: time.Millisecond,
	})
}

func TestBrokerRedactsBeforeReplayAndFansOut(t *testing.T) {
	provider := &fakeProvider{tail: []Entry{{ContainerName: "nopsai-dispatcher", ContainerInstance: "one", Stream: StreamStdout, Line: "password=secret"}}}
	broker := newTestBroker(provider, 10, 2)
	sub, err := broker.Subscribe(context.Background(), "dispatcher", "", 10)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()
	if len(sub.Replay) != 1 || sub.Replay[0].Line != "password=[REDACTED]" || sub.Replay[0].ID == "" {
		t.Fatalf("Subscribe() replay = %#v", sub.Replay)
	}
	provider.emit(t, Entry{ContainerName: "nopsai-dispatcher", ContainerInstance: "one", Stream: StreamStderr, Line: "next"})
	select {
	case entry := <-sub.Entries:
		if entry.Line != "next" || entry.Stream != StreamStderr {
			t.Fatalf("live entry = %#v", entry)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live entry")
	}
	metrics := broker.Metrics().Snapshot()
	if metrics.ActiveStreams != 1 || metrics.OpenedStreams != 1 || metrics.RedactedLines != 1 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestBrokerReplaysAfterCursorAndSignalsExpiredCursor(t *testing.T) {
	provider := &fakeProvider{tail: []Entry{
		{ContainerInstance: "one", Line: "one"}, {ContainerInstance: "one", Line: "two"}, {ContainerInstance: "one", Line: "three"},
	}}
	broker := newTestBroker(provider, 2, 3)
	first, err := broker.Subscribe(context.Background(), "dispatcher", "", 3)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	first.Close()
	if len(first.Replay) != 2 {
		t.Fatalf("replay length = %d, want 2", len(first.Replay))
	}
	second, err := broker.Subscribe(context.Background(), "dispatcher", first.Replay[0].ID, 0)
	if err != nil {
		t.Fatalf("Subscribe(cursor) error = %v", err)
	}
	if len(second.Replay) != 1 || second.Replay[0].Line != "three" {
		t.Fatalf("cursor replay = %#v", second.Replay)
	}
	second.Close()

	oldCursor, _ := broker.codec.Encode(Cursor{SourceID: "dispatcher", Sequence: 1})
	expired, err := broker.Subscribe(context.Background(), "dispatcher", oldCursor, 0)
	if err != nil {
		t.Fatalf("Subscribe(expired) error = %v", err)
	}
	if !expired.Reset {
		t.Fatal("Subscribe(expired) Reset = false, want true")
	}
	expired.Close()
}

func TestBrokerEnforcesSourceAndStreamLimits(t *testing.T) {
	broker := newTestBroker(&fakeProvider{}, 10, 1)
	if _, err := broker.Subscribe(context.Background(), "unknown", "", 0); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("Subscribe(unknown) error = %v", err)
	}
	first, err := broker.Subscribe(context.Background(), "dispatcher", "", 0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if _, err := broker.Subscribe(context.Background(), "dispatcher", "", 0); !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("second Subscribe() error = %v, want ErrStreamLimit", err)
	}
	first.Close()
}

func TestBrokerReturnsProviderTailError(t *testing.T) {
	want := errors.New("docker unavailable")
	broker := newTestBroker(&fakeProvider{tailErr: want}, 10, 1)
	if _, err := broker.Tail(context.Background(), "dispatcher", 10); !errors.Is(err, want) {
		t.Fatalf("Tail() error = %v, want %v", err, want)
	}
	if broker.Metrics().Snapshot().ProviderErrors != 1 {
		t.Fatal("provider error metric was not incremented")
	}
}

func TestBrokerDeduplicatesProviderReconnectBoundary(t *testing.T) {
	emitted := time.Unix(100, 0).UTC()
	provider := &fakeProvider{tail: []Entry{{ContainerInstance: "one", EmittedAt: emitted, Stream: StreamStdout, Line: "same"}}}
	broker := newTestBroker(provider, 10, 1)
	if _, err := broker.Tail(context.Background(), "dispatcher", 10); err != nil {
		t.Fatalf("first Tail() error = %v", err)
	}
	entries, err := broker.Tail(context.Background(), "dispatcher", 10)
	if err != nil {
		t.Fatalf("second Tail() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Tail() entries = %#v, want duplicate boundary collapsed", entries)
	}
}
