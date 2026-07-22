package systemlogs

import (
	"sort"
	"strings"
)

type Source struct {
	ID            string
	DisplayName   string
	ContainerName string
	Optional      bool
}

type Registry struct {
	byID map[string]Source
}

func NewRegistry(sources []Source) *Registry {
	registry := &Registry{byID: make(map[string]Source, len(sources))}
	for _, source := range sources {
		source.ID = strings.TrimSpace(source.ID)
		source.ContainerName = strings.TrimSpace(source.ContainerName)
		if source.ID == "" || source.ContainerName == "" {
			continue
		}
		registry.byID[source.ID] = source
	}
	return registry
}

func DefaultRegistry() *Registry {
	return NewRegistry([]Source{
		{ID: "nopsai", DisplayName: "NopsAI API", ContainerName: "nopsai"},
		{ID: "aaa", DisplayName: "AAA", ContainerName: "nopsai-aaa"},
		{ID: "dispatcher", DisplayName: "Dispatcher", ContainerName: "nopsai-dispatcher"},
		{ID: "git-bot", DisplayName: "Git bot", ContainerName: "nopsai-git-bot"},
		{ID: "ui", DisplayName: "UI", ContainerName: "nopsai-ui"},
		{ID: "docker-socket-proxy", DisplayName: "Docker socket proxy", ContainerName: "nopsai-docker-socket-proxy"},
		{ID: "gotenberg", DisplayName: "Gotenberg", ContainerName: "nopsai-gotenberg"},
		{ID: "db", DisplayName: "Postgres", ContainerName: "nopsai-db"},
		{ID: "docker-runner", DisplayName: "Docker runner", ContainerName: "nopsai-docker-runner", Optional: true},
		{ID: "k8s-runner", DisplayName: "Kubernetes runner", ContainerName: "nopsai-k8s-runner", Optional: true},
	})
}

func (r *Registry) Resolve(id string) (Source, bool) {
	if r == nil {
		return Source{}, false
	}
	source, ok := r.byID[strings.TrimSpace(id)]
	return source, ok
}

func (r *Registry) Sources() []Source {
	if r == nil {
		return nil
	}
	out := make([]Source, 0, len(r.byID))
	for _, source := range r.byID {
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
