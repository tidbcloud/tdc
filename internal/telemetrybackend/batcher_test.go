package telemetrybackend

import (
	"context"
	"testing"
	"time"
)

func TestBatcherFlushesAtEventThresholdAndAttemptsIndependentSinks(t *testing.T) {
	cfg := testConfig()
	cfg.FlushMaxEvents = 2
	failing := newRecordingSink("tidb", errTestSink)
	succeeding := newRecordingSink("posthog", nil)
	metrics := &Metrics{}
	batcher := NewBatcher(cfg, []Sink{failing, succeeding}, discardLogger(), metrics)
	batcher.Start()
	defer closeBatcher(t, batcher)

	if !batcher.Enqueue([]Event{testEvent(), testEvent()}) {
		t.Fatal("Enqueue returned false")
	}
	waitForSink(t, failing)
	waitForSink(t, succeeding)
	if failing.eventCount() != 2 || succeeding.eventCount() != 2 {
		t.Fatalf("sink event counts = %d, %d", failing.eventCount(), succeeding.eventCount())
	}
	if metrics.SinkFailures.Load() != 1 || metrics.SinkSuccesses.Load() != 1 {
		t.Fatalf("sink metrics = failures %d, successes %d", metrics.SinkFailures.Load(), metrics.SinkSuccesses.Load())
	}
}

func TestBatcherAttemptsTiDBWhenPostHogFails(t *testing.T) {
	cfg := testConfig()
	cfg.FlushMaxEvents = 1
	posthog := newRecordingSink("posthog", errTestSink)
	tidb := newRecordingSink("tidb", nil)
	batcher := NewBatcher(cfg, []Sink{posthog, tidb}, discardLogger(), nil)
	batcher.Start()
	defer closeBatcher(t, batcher)

	if !batcher.Enqueue([]Event{testEvent()}) {
		t.Fatal("Enqueue returned false")
	}
	waitForSink(t, posthog)
	waitForSink(t, tidb)
	if tidb.eventCount() != 1 {
		t.Fatalf("TiDB event count = %d, want 1", tidb.eventCount())
	}
}

func TestBatcherFlushesAtByteThreshold(t *testing.T) {
	cfg := testConfig()
	cfg.FlushMaxBytes = 1
	sink := newRecordingSink("sink", nil)
	batcher := NewBatcher(cfg, []Sink{sink}, discardLogger(), nil)
	batcher.Start()
	defer closeBatcher(t, batcher)

	if !batcher.Enqueue([]Event{testEvent()}) {
		t.Fatal("Enqueue returned false")
	}
	waitForSink(t, sink)
}

func TestBatcherFlushesAtInterval(t *testing.T) {
	cfg := testConfig()
	cfg.FlushInterval = 20 * time.Millisecond
	sink := newRecordingSink("sink", nil)
	batcher := NewBatcher(cfg, []Sink{sink}, discardLogger(), nil)
	batcher.Start()
	defer closeBatcher(t, batcher)

	if !batcher.Enqueue([]Event{testEvent()}) {
		t.Fatal("Enqueue returned false")
	}
	waitForSink(t, sink)
}

func TestBatcherShutdownDrainsPendingEvents(t *testing.T) {
	cfg := testConfig()
	sink := newRecordingSink("sink", nil)
	batcher := NewBatcher(cfg, []Sink{sink}, discardLogger(), nil)
	batcher.Start()
	if !batcher.Enqueue([]Event{testEvent()}) {
		t.Fatal("Enqueue returned false")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := batcher.Close(ctx); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if sink.eventCount() != 1 {
		t.Fatalf("sink event count = %d, want 1", sink.eventCount())
	}
}

func TestBatcherShutdownDrainIsBounded(t *testing.T) {
	cfg := testConfig()
	cfg.ShutdownDrainTimeout = 20 * time.Millisecond
	cfg.SinkTimeout = time.Second
	sink := newRecordingSink("sink", nil)
	sink.delay = time.Second
	batcher := NewBatcher(cfg, []Sink{sink}, discardLogger(), nil)
	batcher.Start()
	if !batcher.Enqueue([]Event{testEvent()}) {
		t.Fatal("Enqueue returned false")
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := batcher.Close(ctx); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 300*time.Millisecond {
		t.Fatalf("Close took %v, expected bounded shutdown", elapsed)
	}
}

func TestBatcherRejectsRequestAtomicallyWhenFull(t *testing.T) {
	cfg := testConfig()
	cfg.BufferMaxEvents = 1
	cfg.FlushMaxEvents = 1
	batcher := NewBatcher(cfg, nil, discardLogger(), nil)
	if batcher.Enqueue([]Event{testEvent(), testEvent()}) {
		t.Fatal("oversized enqueue was accepted")
	}
	if batcher.Pending() != 0 {
		t.Fatalf("Pending = %d, want 0", batcher.Pending())
	}
}

func closeBatcher(t *testing.T, batcher *Batcher) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := batcher.Close(ctx); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func waitForSink(t *testing.T, sink *recordingSink) {
	t.Helper()
	select {
	case <-sink.called:
	case <-time.After(time.Second):
		t.Fatalf("sink %s was not called", sink.name)
	}
}
