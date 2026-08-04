package systemlogs

import (
	"context"
	"errors"
)

type MultiProvider struct {
	providers []Provider
}

func NewMultiProvider(providers ...Provider) *MultiProvider {
	out := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			out = append(out, provider)
		}
	}
	return &MultiProvider{providers: out}
}

func (p *MultiProvider) ListSources(ctx context.Context) ([]SourceStatus, error) {
	if p == nil || len(p.providers) == 0 {
		return nil, ErrSourceNotFound
	}
	merged := map[string]SourceStatus{}
	order := make([]string, 0)
	var firstErr error
	success := false
	for _, provider := range p.providers {
		sources, err := provider.ListSources(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		success = true
		for _, source := range sources {
			if source.ID == "" {
				continue
			}
			existing, ok := merged[source.ID]
			if !ok {
				order = append(order, source.ID)
				merged[source.ID] = source
				continue
			}
			if !existing.Available && source.Available {
				merged[source.ID] = source
			}
		}
	}
	if !success {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, ErrSourceNotFound
	}
	out := make([]SourceStatus, 0, len(order))
	for _, id := range order {
		out = append(out, merged[id])
	}
	return out, nil
}

func (p *MultiProvider) Tail(ctx context.Context, sourceID string, lines int) ([]Entry, error) {
	var firstErr error
	for _, provider := range p.providers {
		entries, err := provider.Tail(ctx, sourceID, lines)
		if err == nil {
			return entries, nil
		}
		if errors.Is(err, ErrSourceNotFound) {
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrSourceNotFound
}

func (p *MultiProvider) Follow(ctx context.Context, sourceID string, after Cursor, emit func(Entry)) error {
	var firstErr error
	for _, provider := range p.providers {
		err := provider.Follow(ctx, sourceID, after, emit)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrSourceNotFound) {
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return ErrSourceNotFound
}
