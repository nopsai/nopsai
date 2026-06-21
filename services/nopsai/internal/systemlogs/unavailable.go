package systemlogs

import (
	"context"
	"fmt"
)

type UnavailableProvider struct {
	registry *Registry
	reason   string
}

func NewUnavailableProvider(registry *Registry, reason string) *UnavailableProvider {
	return &UnavailableProvider{registry: registry, reason: reason}
}

func (p *UnavailableProvider) ListSources(context.Context) ([]SourceStatus, error) {
	sources := p.registry.Sources()
	out := make([]SourceStatus, 0, len(sources))
	for _, source := range sources {
		out = append(out, SourceStatus{
			ID: source.ID, DisplayName: source.DisplayName, ContainerName: source.ContainerName,
			State: "unavailable", Status: p.reason,
		})
	}
	return out, nil
}

func (p *UnavailableProvider) Tail(context.Context, string, int) ([]Entry, error) {
	return nil, fmt.Errorf("system log provider unavailable: %s", p.reason)
}

func (p *UnavailableProvider) Follow(context.Context, string, Cursor, func(Entry)) error {
	return fmt.Errorf("system log provider unavailable: %s", p.reason)
}
