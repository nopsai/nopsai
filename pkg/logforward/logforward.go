package logforward

import (
	"bufio"
	"context"
	"io"
	"time"
	"unicode/utf8"
)

const (
	DefaultBatchSize           = 50
	DefaultBatchTimeout        = 500 * time.Millisecond
	DefaultScannerBufferBytes  = 64 * 1024
	DefaultMaxScanTokenBytes   = 16 * 1024 * 1024
	DefaultMaxForwardLineBytes = 60 * 1024
)

type Options struct {
	BatchSize           int
	BatchTimeout        time.Duration
	ScannerBufferBytes  int
	MaxScanTokenBytes   int
	MaxForwardLineBytes int
	OnScannerError      func(error)
}

type Sender func(ctx context.Context, lines []string)

func Forward(ctx context.Context, reader io.Reader, send Sender, opts Options) {
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil || send == nil {
		return
	}
	opts = normalizeOptions(opts)

	logChan := make(chan string, opts.BatchSize*2)
	go scanLines(ctx, reader, logChan, opts)

	ticker := time.NewTicker(opts.BatchTimeout)
	defer ticker.Stop()

	var batch []string
	appendLine := func(sendCtx context.Context, line string) {
		for _, chunk := range splitLine(line, opts.MaxForwardLineBytes) {
			batch = append(batch, chunk)
			if len(batch) >= opts.BatchSize {
				send(sendCtx, batch)
				batch = nil
			}
		}
	}
	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		send(flushCtx, batch)
		batch = nil
	}
	drainReadyLines := func() {
		for {
			select {
			case line, ok := <-logChan:
				if !ok {
					flush(context.Background())
					return
				}
				appendLine(context.Background(), line)
			default:
				flush(context.Background())
				return
			}
		}
	}

	for {
		select {
		case line, ok := <-logChan:
			if !ok {
				flush(context.Background())
				return
			}
			appendLine(ctx, line)
		case <-ticker.C:
			flush(ctx)
		case <-ctx.Done():
			drainReadyLines()
			return
		}
	}
}

func normalizeOptions(opts Options) Options {
	if opts.BatchSize <= 0 {
		opts.BatchSize = DefaultBatchSize
	}
	if opts.BatchTimeout <= 0 {
		opts.BatchTimeout = DefaultBatchTimeout
	}
	if opts.ScannerBufferBytes <= 0 {
		opts.ScannerBufferBytes = DefaultScannerBufferBytes
	}
	if opts.MaxScanTokenBytes <= 0 {
		opts.MaxScanTokenBytes = DefaultMaxScanTokenBytes
	}
	if opts.MaxForwardLineBytes <= 0 {
		opts.MaxForwardLineBytes = DefaultMaxForwardLineBytes
	}
	if opts.ScannerBufferBytes > opts.MaxScanTokenBytes {
		opts.ScannerBufferBytes = opts.MaxScanTokenBytes
	}
	return opts
}

func scanLines(ctx context.Context, reader io.Reader, out chan<- string, opts Options) {
	defer close(out)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, opts.ScannerBufferBytes), opts.MaxScanTokenBytes)
	for scanner.Scan() {
		line := scanner.Text()
		select {
		case out <- line:
		case <-ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil && opts.OnScannerError != nil {
		opts.OnScannerError(err)
	}
}

func splitLine(line string, maxBytes int) []string {
	if maxBytes <= 0 || len(line) <= maxBytes {
		return []string{line}
	}

	chunks := make([]string, 0, len(line)/maxBytes+1)
	for len(line) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		if cut == 0 {
			cut = maxBytes
		}
		chunks = append(chunks, line[:cut])
		line = line[cut:]
	}
	if line != "" {
		chunks = append(chunks, line)
	}
	return chunks
}
