package logforward

import (
	"bufio"
	"context"
	"strings"
	"testing"
	"time"
)

func TestForwardAllowsLinesAboveScannerDefault(t *testing.T) {
	longLine := strings.Repeat("x", bufio.MaxScanTokenSize+1024)
	var got []string

	Forward(context.Background(), strings.NewReader(longLine+"\ntail\n"), func(_ context.Context, lines []string) {
		got = append(got, lines...)
	}, Options{BatchTimeout: time.Hour})

	if len(got) < 2 {
		t.Fatalf("Forward() emitted %d lines, want split long line and tail", len(got))
	}
	if got[len(got)-1] != "tail" {
		t.Fatalf("last forwarded line = %q, want tail", got[len(got)-1])
	}
	if strings.Join(got[:len(got)-1], "") != longLine {
		t.Fatal("forwarded long line chunks did not reconstruct the original line")
	}
}

func TestForwardSplitsAtRuneBoundary(t *testing.T) {
	line := strings.Repeat("a", 9) + "é" + strings.Repeat("b", 9)
	var got []string

	Forward(context.Background(), strings.NewReader(line+"\n"), func(_ context.Context, lines []string) {
		got = append(got, lines...)
	}, Options{BatchTimeout: time.Hour, MaxForwardLineBytes: 10})

	if strings.Join(got, "") != line {
		t.Fatal("forwarded chunks did not reconstruct the original UTF-8 line")
	}
	for _, chunk := range got {
		if strings.ToValidUTF8(chunk, "") != chunk {
			t.Fatalf("chunk is not valid UTF-8: %q", chunk)
		}
	}
}

func TestForwardFiltersLinesBeforeChunking(t *testing.T) {
	var got []string
	var filtered []string
	longLine := strings.Repeat("x", 12)

	Forward(context.Background(), strings.NewReader("drop\n"+longLine+"\n"), func(_ context.Context, lines []string) {
		got = append(got, lines...)
	}, Options{
		BatchTimeout:        time.Hour,
		MaxForwardLineBytes: 5,
		FilterLine: func(line string) (string, bool) {
			filtered = append(filtered, line)
			if line == "drop" {
				return "", false
			}
			return line, true
		},
	})

	if strings.Join(filtered, ",") != "drop,"+longLine {
		t.Fatalf("filtered lines = %#v, want full raw lines before chunking", filtered)
	}
	if strings.Join(got, "") != longLine {
		t.Fatalf("forwarded chunks = %#v, want only long line chunks", got)
	}
}

func TestForwardFlushesPartialBatchOnEOF(t *testing.T) {
	var got [][]string

	Forward(context.Background(), strings.NewReader("one\ntwo\n"), func(_ context.Context, lines []string) {
		got = append(got, append([]string(nil), lines...))
	}, Options{BatchSize: 10, BatchTimeout: time.Hour})

	if len(got) != 1 {
		t.Fatalf("Forward() batches = %d, want 1", len(got))
	}
	if strings.Join(got[0], ",") != "one,two" {
		t.Fatalf("Forward() batch = %#v, want one,two", got[0])
	}
}
