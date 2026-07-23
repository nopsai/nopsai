package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"nopsai/services/nopsai/internal/systemlogs"
)

type fakeDocker struct {
	containers []ContainerSummary
	tty        bool
	logs       []byte
	options    client.ContainerLogsOptions
	logTarget  string
}

func (f *fakeDocker) ListContainers(context.Context) ([]ContainerSummary, error) {
	return f.containers, nil
}
func (f *fakeDocker) ContainerTTY(context.Context, string) (bool, error) { return f.tty, nil }
func (f *fakeDocker) ContainerLogs(_ context.Context, containerID string, options client.ContainerLogsOptions) (io.ReadCloser, error) {
	f.options = options
	f.logTarget = containerID
	return io.NopCloser(bytes.NewReader(f.logs)), nil
}

func dockerFrame(stream byte, payload string) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	return append(header, []byte(payload)...)
}

func TestProviderListsRegistrySourcesWithoutExposingOtherContainers(t *testing.T) {
	fake := &fakeDocker{containers: []ContainerSummary{
		{ID: "dispatcher-id", Names: []string{"/nopsai-dispatcher"}, State: "running", Health: "healthy"},
		{ID: "database-id", Names: []string{"/nopsai-db"}, State: "running"},
	}}
	provider := NewProvider(fake, systemlogs.NewRegistry([]systemlogs.Source{{ID: "dispatcher", DisplayName: "Dispatcher", ContainerName: "nopsai-dispatcher"}}))
	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 || sources[0].ID != "dispatcher" || sources[0].ContainerInstance != "dispatcher-id" || !sources[0].Available {
		t.Fatalf("ListSources() = %#v", sources)
	}
}

func TestProviderDoesNotExposeDockerHealthNone(t *testing.T) {
	fake := &fakeDocker{containers: []ContainerSummary{
		{ID: "aaa-id", Names: []string{"/nopsai-aaa"}, State: "running", Health: "none", Status: "Up 2 minutes"},
	}}
	provider := NewProvider(fake, systemlogs.NewRegistry([]systemlogs.Source{{ID: "aaa", DisplayName: "AAA", ContainerName: "nopsai-aaa"}}))
	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 || !sources[0].Available || sources[0].State != "running" {
		t.Fatalf("ListSources() = %#v, want running available source", sources)
	}
	if sources[0].Health != "" {
		t.Fatalf("source health = %q, want empty health when Docker reports none", sources[0].Health)
	}
}

func TestProviderTailPreservesMultiplexedStreamsAndTimestamps(t *testing.T) {
	stdout := "2026-06-21T12:00:00.123456789Z hello\n"
	stderr := "2026-06-21T12:00:01Z failed\n"
	fake := &fakeDocker{
		containers: []ContainerSummary{{ID: "instance", Names: []string{"/nopsai-dispatcher"}}},
		logs:       append(dockerFrame(1, stdout), dockerFrame(2, stderr)...),
	}
	provider := NewProvider(fake, systemlogs.NewRegistry([]systemlogs.Source{{ID: "dispatcher", ContainerName: "nopsai-dispatcher"}}))
	entries, err := provider.Tail(context.Background(), "dispatcher", 25)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Stream != systemlogs.StreamStdout || entries[1].Stream != systemlogs.StreamStderr {
		t.Fatalf("Tail() entries = %#v", entries)
	}
	if entries[0].Line != "hello" || !entries[0].EmittedAt.Equal(time.Date(2026, 6, 21, 12, 0, 0, 123456789, time.UTC)) {
		t.Fatalf("Tail() first entry = %#v", entries[0])
	}
	if fake.options.Tail != "25" || !fake.options.Timestamps || fake.options.Follow {
		t.Fatalf("Tail() options = %#v", fake.options)
	}
	if fake.logTarget != "nopsai-dispatcher" {
		t.Fatalf("Tail() Docker target = %q, want allow-listed container name", fake.logTarget)
	}
}

func TestProviderTailHandlesTTYAndRejectsUnknownSource(t *testing.T) {
	fake := &fakeDocker{tty: true, containers: []ContainerSummary{{ID: "instance", Names: []string{"/nopsai-ui"}}}, logs: []byte("plain line\n")}
	provider := NewProvider(fake, systemlogs.NewRegistry([]systemlogs.Source{{ID: "ui", ContainerName: "nopsai-ui"}}))
	provider.now = func() time.Time { return time.Unix(20, 0).UTC() }
	entries, err := provider.Tail(context.Background(), "ui", 1)
	if err != nil || len(entries) != 1 || entries[0].Line != "plain line" || entries[0].Stream != systemlogs.StreamStdout {
		t.Fatalf("Tail(TTY) = %#v, %v", entries, err)
	}
	if _, err := provider.Tail(context.Background(), "base", 1); err != systemlogs.ErrSourceNotFound {
		t.Fatalf("Tail(unknown) error = %v", err)
	}
}

func TestDecodeDockerLogsJoinsSplitFrames(t *testing.T) {
	data := append(dockerFrame(1, "partial"), dockerFrame(1, " line\nnext")...)
	var lines []string
	err := decodeDockerLogs(bytes.NewReader(data), false, func(_ systemlogs.Stream, line string) { lines = append(lines, line) })
	if err != nil {
		t.Fatalf("decodeDockerLogs() error = %v", err)
	}
	if len(lines) != 2 || lines[0] != "partial line" || lines[1] != "next" {
		t.Fatalf("lines = %#v", lines)
	}
}
