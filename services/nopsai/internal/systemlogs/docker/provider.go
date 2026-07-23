package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"

	"nopsai/services/nopsai/internal/systemlogs"
)

const maxDockerFrameBytes = 32 * 1024 * 1024

type ContainerSummary struct {
	ID     string
	Names  []string
	State  string
	Status string
	Health string
}

type DockerAPI interface {
	ListContainers(ctx context.Context) ([]ContainerSummary, error)
	ContainerTTY(ctx context.Context, containerID string) (bool, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (io.ReadCloser, error)
}

type Provider struct {
	docker   DockerAPI
	registry *systemlogs.Registry
	now      func() time.Time
}

func NewProvider(api DockerAPI, registry *systemlogs.Registry) *Provider {
	if registry == nil {
		registry = systemlogs.DefaultRegistry()
	}
	return &Provider{docker: api, registry: registry, now: time.Now}
}

func NewMobyProvider(dockerClient *client.Client, registry *systemlogs.Registry) *Provider {
	return NewProvider(&mobyAPI{client: dockerClient}, registry)
}

func (p *Provider) ListSources(ctx context.Context) ([]systemlogs.SourceStatus, error) {
	containers, err := p.docker.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]ContainerSummary, len(containers))
	for _, candidate := range containers {
		for _, name := range candidate.Names {
			byName[strings.TrimPrefix(name, "/")] = candidate
		}
	}
	sources := p.registry.Sources()
	out := make([]systemlogs.SourceStatus, 0, len(sources))
	for _, source := range sources {
		status := systemlogs.SourceStatus{
			ID: source.ID, DisplayName: source.DisplayName, ContainerName: source.ContainerName,
			State: "unavailable",
		}
		if candidate, ok := byName[source.ContainerName]; ok {
			status.ContainerInstance = candidate.ID
			status.Available = true
			status.State = candidate.State
			status.Health = normalizeDockerHealth(candidate.Health)
			status.Status = candidate.Status
		}
		out = append(out, status)
	}
	return out, nil
}

func normalizeDockerHealth(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") {
		return ""
	}
	return value
}

func (p *Provider) Tail(ctx context.Context, sourceID string, lines int) ([]systemlogs.Entry, error) {
	source, containerInfo, err := p.resolve(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if lines <= 0 {
		lines = 500
	}
	reader, tty, err := p.logs(ctx, source.ContainerName, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: strconv.Itoa(lines),
	})
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	entries := make([]systemlogs.Entry, 0, lines)
	err = decodeDockerLogs(reader, tty, func(stream systemlogs.Stream, line string) {
		entries = append(entries, p.entry(source, containerInfo.ID, stream, line))
	})
	return entries, err
}

func (p *Provider) Follow(ctx context.Context, sourceID string, after systemlogs.Cursor, emit func(systemlogs.Entry)) error {
	source, containerInfo, err := p.resolve(ctx, sourceID)
	if err != nil {
		return err
	}
	options := client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Timestamps: true, Follow: true, Tail: "0"}
	if !after.EmittedAt.IsZero() {
		options.Since = after.EmittedAt.UTC().Format(time.RFC3339Nano)
	}
	reader, tty, err := p.logs(ctx, source.ContainerName, options)
	if err != nil {
		return err
	}
	defer reader.Close()
	return decodeDockerLogs(reader, tty, func(stream systemlogs.Stream, line string) {
		emit(p.entry(source, containerInfo.ID, stream, line))
	})
}

func (p *Provider) resolve(ctx context.Context, sourceID string) (systemlogs.Source, ContainerSummary, error) {
	source, ok := p.registry.Resolve(sourceID)
	if !ok {
		return systemlogs.Source{}, ContainerSummary{}, systemlogs.ErrSourceNotFound
	}
	containers, err := p.docker.ListContainers(ctx)
	if err != nil {
		return systemlogs.Source{}, ContainerSummary{}, err
	}
	for _, candidate := range containers {
		for _, name := range candidate.Names {
			if strings.TrimPrefix(name, "/") == source.ContainerName {
				return source, candidate, nil
			}
		}
	}
	return systemlogs.Source{}, ContainerSummary{}, systemlogs.ErrSourceNotFound
}

func (p *Provider) logs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (io.ReadCloser, bool, error) {
	tty, err := p.docker.ContainerTTY(ctx, containerID)
	if err != nil {
		return nil, false, err
	}
	reader, err := p.docker.ContainerLogs(ctx, containerID, options)
	return reader, tty, err
}

func (p *Provider) entry(source systemlogs.Source, instance string, stream systemlogs.Stream, raw string) systemlogs.Entry {
	emittedAt, line := splitTimestamp(raw, p.now().UTC())
	return systemlogs.Entry{
		SourceID: source.ID, ContainerName: source.ContainerName, ContainerInstance: instance,
		EmittedAt: emittedAt, Stream: stream, Line: line,
	}
}

func splitTimestamp(raw string, fallback time.Time) (time.Time, string) {
	timestamp, line, found := strings.Cut(strings.TrimRight(raw, "\r\n"), " ")
	if !found {
		return fallback, timestamp
	}
	emittedAt, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return fallback, strings.TrimRight(raw, "\r\n")
	}
	return emittedAt.UTC(), line
}

func decodeDockerLogs(reader io.Reader, tty bool, emit func(systemlogs.Stream, string)) error {
	if tty {
		return scanLines(reader, func(line string) { emit(systemlogs.StreamStdout, line) })
	}
	pending := map[systemlogs.Stream]string{systemlogs.StreamStdout: "", systemlogs.StreamStderr: ""}
	flushPayload := func(stream systemlogs.Stream, payload string, final bool) {
		value := pending[stream] + payload
		parts := strings.Split(value, "\n")
		limit := len(parts) - 1
		if final && parts[len(parts)-1] != "" {
			limit = len(parts)
		}
		for _, line := range parts[:limit] {
			emit(stream, strings.TrimSuffix(line, "\r"))
		}
		if limit < len(parts) {
			pending[stream] = parts[len(parts)-1]
		} else {
			pending[stream] = ""
		}
	}

	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(reader, header)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			for stream, value := range pending {
				if value != "" {
					flushPayload(stream, "", true)
				}
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("read docker log frame header: %w", err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read docker log frame header: %w", err)
		}
		size := binary.BigEndian.Uint32(header[4:])
		if size > maxDockerFrameBytes {
			return fmt.Errorf("docker log frame exceeds %d bytes", maxDockerFrameBytes)
		}
		payload := make([]byte, int(size))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return fmt.Errorf("read docker log frame: %w", err)
		}
		stream := systemlogs.StreamStdout
		if header[0] == 2 {
			stream = systemlogs.StreamStderr
		}
		flushPayload(stream, string(payload), false)
	}
}

func scanLines(reader io.Reader, emit func(string)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxDockerFrameBytes)
	for scanner.Scan() {
		emit(scanner.Text())
	}
	return scanner.Err()
}

type mobyAPI struct{ client *client.Client }

func (m *mobyAPI) ListContainers(ctx context.Context) ([]ContainerSummary, error) {
	result, err := m.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]ContainerSummary, 0, len(result.Items))
	for _, item := range result.Items {
		health := ""
		if item.Health != nil {
			health = string(item.Health.Status)
		}
		out = append(out, ContainerSummary{ID: item.ID, Names: item.Names, State: string(item.State), Status: item.Status, Health: health})
	}
	return out, nil
}

func (m *mobyAPI) ContainerTTY(ctx context.Context, containerID string) (bool, error) {
	result, err := m.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return false, err
	}
	return result.Container.Config != nil && result.Container.Config.Tty, nil
}

func (m *mobyAPI) ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (io.ReadCloser, error) {
	return m.client.ContainerLogs(ctx, containerID, options)
}
