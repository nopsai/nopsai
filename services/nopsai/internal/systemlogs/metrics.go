package systemlogs

import "sync/atomic"

type Metrics struct {
	ActiveStreams  atomic.Int64
	OpenedStreams  atomic.Uint64
	Reconnects     atomic.Uint64
	RedactedLines  atomic.Uint64
	DroppedLines   atomic.Uint64
	ProviderErrors atomic.Uint64
}

type MetricsSnapshot struct {
	ActiveStreams  int64
	OpenedStreams  uint64
	Reconnects     uint64
	RedactedLines  uint64
	DroppedLines   uint64
	ProviderErrors uint64
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		ActiveStreams:  m.ActiveStreams.Load(),
		OpenedStreams:  m.OpenedStreams.Load(),
		Reconnects:     m.Reconnects.Load(),
		RedactedLines:  m.RedactedLines.Load(),
		DroppedLines:   m.DroppedLines.Load(),
		ProviderErrors: m.ProviderErrors.Load(),
	}
}
