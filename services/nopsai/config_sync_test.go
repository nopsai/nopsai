package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartConfigSyncAllowsOnlyOneConcurrentStart(t *testing.T) {
	var (
		app         App
		successes   atomic.Int32
		wg          sync.WaitGroup
		concurrency = 32
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			startedAt := time.Unix(int64(offset+1), 0)
			if _, ok := app.startConfigSync(startedAt); ok {
				successes.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly one config sync start to succeed, got %d", got)
	}

	status := app.getConfigSyncStatus()
	if status.Status != "running" {
		t.Fatalf("expected config sync status to be running, got %q", status.Status)
	}
	if status.StartedAt == nil {
		t.Fatal("expected config sync start time to be recorded")
	}
}
