package systemlogs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type BrokerOptions struct {
	BufferLines         int
	BufferAge           time.Duration
	MaxTailLines        int
	MaxStreams          int
	MaxStreamsPerSource int
	RetryDelay          time.Duration
	Now                 func() time.Time
}

type Broker struct {
	provider Provider
	registry *Registry
	redactor *Redactor
	codec    *CursorCodec
	options  BrokerOptions
	metrics  *Metrics

	mu             sync.Mutex
	sources        map[string]*sourceBroker
	activeStreams  int
	nextSubscriber uint64
	nextSequence   atomic.Uint64
}

type sourceBroker struct {
	mu          sync.Mutex
	buffer      *ringBuffer
	subscribers map[uint64]chan Entry
	recent      map[string]struct{}
	recentOrder []string
	recentLimit int
	cancel      context.CancelFunc
	collecting  bool
	lastCursor  Cursor
}

type Subscription struct {
	Entries <-chan Entry
	Replay  []Entry
	Reset   bool
	close   func()
	once    sync.Once
}

func (s *Subscription) Close() {
	if s != nil {
		s.once.Do(s.close)
	}
}

func NewBroker(provider Provider, registry *Registry, redactor *Redactor, codec *CursorCodec, options BrokerOptions) *Broker {
	if options.BufferLines <= 0 {
		options.BufferLines = 10_000
	}
	if options.BufferAge <= 0 {
		options.BufferAge = 15 * time.Minute
	}
	if options.MaxTailLines <= 0 {
		options.MaxTailLines = 2_000
	}
	if options.MaxStreams <= 0 {
		options.MaxStreams = 20
	}
	if options.MaxStreamsPerSource <= 0 {
		options.MaxStreamsPerSource = 10
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = time.Second
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if registry == nil {
		registry = DefaultRegistry()
	}
	if redactor == nil {
		redactor = NewRedactor(16 * 1024)
	}
	if codec == nil {
		codec = NewCursorCodec([]byte("system-logs-development-cursor-key"))
	}
	return &Broker{
		provider: provider, registry: registry, redactor: redactor, codec: codec,
		options: options, metrics: &Metrics{}, sources: make(map[string]*sourceBroker),
	}
}

func (b *Broker) Registry() *Registry { return b.registry }
func (b *Broker) Metrics() *Metrics   { return b.metrics }
func (b *Broker) MaxTailLines() int   { return b.options.MaxTailLines }

func (b *Broker) ListSources(ctx context.Context) ([]SourceStatus, error) {
	if b == nil || b.provider == nil {
		return nil, errors.New("system log provider unavailable")
	}
	return b.provider.ListSources(ctx)
}

func (b *Broker) Tail(ctx context.Context, sourceID string, lines int) ([]Entry, error) {
	sourceID = strings.TrimSpace(sourceID)
	if !b.sourceKnown(sourceID) {
		return nil, ErrSourceNotFound
	}
	lines = b.clampTail(lines)
	entries, err := b.provider.Tail(ctx, sourceID, lines)
	if err != nil {
		b.metrics.ProviderErrors.Add(1)
		return nil, err
	}
	for _, entry := range entries {
		b.publish(sourceID, entry)
	}
	state := b.source(sourceID)
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.buffer.tail(lines, b.options.Now()), nil
}

func (b *Broker) Subscribe(ctx context.Context, sourceID, token string, tail int) (*Subscription, error) {
	sourceID = strings.TrimSpace(sourceID)
	if !b.sourceKnown(sourceID) {
		return nil, ErrSourceNotFound
	}
	state := b.source(sourceID)
	if strings.TrimSpace(token) == "" {
		state.mu.Lock()
		empty := len(state.buffer.entries) == 0
		state.mu.Unlock()
		if empty && tail > 0 {
			if _, err := b.Tail(ctx, sourceID, tail); err != nil {
				return nil, err
			}
		}
	}

	b.mu.Lock()
	state.mu.Lock()
	if b.activeStreams >= b.options.MaxStreams || len(state.subscribers) >= b.options.MaxStreamsPerSource {
		state.mu.Unlock()
		b.mu.Unlock()
		return nil, ErrStreamLimit
	}
	b.nextSubscriber++
	id := b.nextSubscriber
	channel := make(chan Entry, b.options.MaxTailLines+256)
	replay, reset, err := b.replayLocked(state, sourceID, token, tail)
	if err != nil {
		state.mu.Unlock()
		b.mu.Unlock()
		return nil, err
	}
	state.subscribers[id] = channel
	b.activeStreams++
	b.metrics.ActiveStreams.Add(1)
	b.metrics.OpenedStreams.Add(1)
	if !state.collecting {
		collectorCtx, cancel := context.WithCancel(context.Background())
		state.cancel = cancel
		state.collecting = true
		go b.collect(collectorCtx, sourceID, state)
	}
	state.mu.Unlock()
	b.mu.Unlock()

	subscription := &Subscription{Entries: channel, Replay: replay, Reset: reset}
	subscription.close = func() { b.unsubscribe(sourceID, id) }
	return subscription, nil
}

func (b *Broker) sourceKnown(sourceID string) bool {
	if sourceID == "" {
		return false
	}
	if _, ok := b.registry.Resolve(sourceID); ok {
		return true
	}
	_, ok := ParseRunnerSourceID(sourceID)
	return ok
}

func (b *Broker) replayLocked(state *sourceBroker, sourceID, token string, tail int) ([]Entry, bool, error) {
	if strings.TrimSpace(token) == "" {
		return state.buffer.tail(b.clampTail(tail), b.options.Now()), false, nil
	}
	cursor, err := b.codec.Decode(token)
	if err != nil || cursor.SourceID != sourceID {
		return nil, false, ErrCursorInvalid
	}
	replay, err := state.buffer.after(cursor.Sequence, b.options.Now())
	if errors.Is(err, ErrCursorExpired) {
		return nil, true, nil
	}
	return replay, false, err
}

func (b *Broker) source(sourceID string) *sourceBroker {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sourceLocked(sourceID)
}

func (b *Broker) sourceLocked(sourceID string) *sourceBroker {
	state := b.sources[sourceID]
	if state == nil {
		state = &sourceBroker{buffer: newRingBuffer(b.options.BufferLines, b.options.BufferAge), subscribers: make(map[uint64]chan Entry)}
		state.recent = make(map[string]struct{}, b.options.BufferLines)
		state.recentLimit = b.options.BufferLines
		b.sources[sourceID] = state
	}
	return state
}

func (b *Broker) publish(sourceID string, entry Entry) {
	state := b.source(sourceID)
	now := b.options.Now().UTC()
	entry.SourceID = sourceID
	entry.ObservedAt = now
	if entry.EmittedAt.IsZero() {
		entry.EmittedAt = now
	}
	original := entry.Line
	entry.Line = b.redactor.Redact(entry.Line)
	if original != entry.Line {
		b.metrics.RedactedLines.Add(1)
	}
	state.mu.Lock()
	fingerprint := entry.ContainerInstance + "\x00" + entry.EmittedAt.UTC().Format(time.RFC3339Nano) + "\x00" + string(entry.Stream) + "\x00" + entry.Line
	if _, exists := state.recent[fingerprint]; exists {
		state.mu.Unlock()
		return
	}
	state.recent[fingerprint] = struct{}{}
	state.recentOrder = append(state.recentOrder, fingerprint)
	if overflow := len(state.recentOrder) - state.recentLimit; overflow > 0 {
		for _, expired := range state.recentOrder[:overflow] {
			delete(state.recent, expired)
		}
		state.recentOrder = append(state.recentOrder[:0], state.recentOrder[overflow:]...)
	}
	sequence := b.nextSequence.Add(1)
	entry.sequence = sequence
	cursor := Cursor{SourceID: sourceID, ContainerInstance: entry.ContainerInstance, Sequence: sequence, EmittedAt: entry.EmittedAt}
	entry.ID, _ = b.codec.Encode(cursor)
	state.buffer.append(entry, now)
	state.lastCursor = cursor
	for _, subscriber := range state.subscribers {
		select {
		case subscriber <- entry:
		default:
			b.metrics.DroppedLines.Add(1)
		}
	}
	state.mu.Unlock()
}

func (b *Broker) collect(ctx context.Context, sourceID string, state *sourceBroker) {
	for {
		state.mu.Lock()
		after := state.lastCursor
		state.mu.Unlock()
		err := b.provider.Follow(ctx, sourceID, after, func(entry Entry) { b.publish(sourceID, entry) })
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			b.metrics.ProviderErrors.Add(1)
		}
		b.metrics.Reconnects.Add(1)
		timer := time.NewTimer(b.options.RetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (b *Broker) unsubscribe(sourceID string, id uint64) {
	b.mu.Lock()
	state := b.sourceLocked(sourceID)
	state.mu.Lock()
	if channel, ok := state.subscribers[id]; ok {
		delete(state.subscribers, id)
		close(channel)
		b.activeStreams--
		b.metrics.ActiveStreams.Add(-1)
	}
	if len(state.subscribers) == 0 && state.collecting {
		state.cancel()
		state.cancel = nil
		state.collecting = false
	}
	state.mu.Unlock()
	b.mu.Unlock()
}

func (b *Broker) clampTail(lines int) int {
	if lines <= 0 {
		return 500
	}
	if lines > b.options.MaxTailLines {
		return b.options.MaxTailLines
	}
	return lines
}
