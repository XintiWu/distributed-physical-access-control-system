package queue

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/tsmc/access-api/internal/model"
)

type MockKafkaWriter struct {
	WriteMessagesFn func(ctx context.Context, msgs ...kafka.Message) error
	CloseFn         func() error
}

func (m *MockKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if m.WriteMessagesFn != nil {
		return m.WriteMessagesFn(ctx, msgs...)
	}
	return nil
}

func (m *MockKafkaWriter) Close() error {
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}

func TestNewKafkaProducer(t *testing.T) {
	brokers := []string{"localhost:9092"}
	topic := "inout-events"
	p := NewKafkaProducer(brokers, topic)
	if p == nil {
		t.Fatal("expected producer to be non-nil")
	}
	// Wait a tiny bit for retryLoop to start and close it.
	time.Sleep(5 * time.Millisecond)
	p.Close()
}

func TestKafkaProducer_Publish_Success(t *testing.T) {
	mockWriter := &MockKafkaWriter{}
	p := &KafkaProducer{
		writer: mockWriter,
		retry:  make(chan model.InOutEvent, 10),
		done:   make(chan struct{}),
	}
	p.wg.Add(1)
	go p.retryLoop()
	defer p.Close()

	ev := model.InOutEvent{EventID: "evt-ok"}
	err := p.Publish(context.Background(), ev)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestKafkaProducer_Publish_Failure_QueueRetry(t *testing.T) {
	writeErr := errors.New("kafka connection timed out")
	mockWriter := &MockKafkaWriter{
		WriteMessagesFn: func(ctx context.Context, msgs ...kafka.Message) error {
			return writeErr
		},
	}
	p := &KafkaProducer{
		writer: mockWriter,
		retry:  make(chan model.InOutEvent, 10),
		done:   make(chan struct{}),
	}
	p.wg.Add(1)
	go p.retryLoop()
	defer p.Close()

	ev := model.InOutEvent{EventID: "evt-fail"}
	err := p.Publish(context.Background(), ev)
	if !errors.Is(err, writeErr) {
		t.Fatalf("expected write error, got %v", err)
	}

	// Message should be queued in retry channel
	select {
	case retriedEvent := <-p.retry:
		if retriedEvent.EventID != "evt-fail" {
			t.Errorf("unexpected event ID: %s", retriedEvent.EventID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected event to be queued for retry")
	}
}

func TestKafkaProducer_Publish_QueueFull_OutboxAppend(t *testing.T) {
	writeErr := errors.New("write failed")
	mockWriter := &MockKafkaWriter{
		WriteMessagesFn: func(ctx context.Context, msgs ...kafka.Message) error {
			return writeErr
		},
	}
	ob, err := NewFileOutbox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	p := &KafkaProducer{
		writer: mockWriter,
		retry:  make(chan model.InOutEvent, 1),
		outbox: ob,
		done:   make(chan struct{}),
	}
	p.wg.Add(1)
	go p.retryLoop()
	defer p.Close()

	// Fill the retry queue
	p.retry <- model.InOutEvent{EventID: "blocker"}

	ev := model.InOutEvent{EventID: "spooled"}
	err = p.Publish(context.Background(), ev)
	// Should return nil because it succeeded in spooling to outbox
	if err != nil {
		t.Fatalf("expected nil since it was spooled to outbox, got %v", err)
	}

	// Let's verify outbox has the event.
	// ReplayOutbox might fail because mockWriter is still failing, but we just want to ensure it executes.
	_, _ = p.ReplayOutbox(context.Background())
}

func TestFileOutbox_InvalidPathDirCreation(t *testing.T) {
	// MkdirAll on a path that is already a file
	tmpFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(tmpFile, []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewFileOutbox(tmpFile)
	if err == nil {
		t.Error("expected error creating outbox in a path that is a file")
	}
}

func TestFileOutbox_AppendToDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	ob := &FileOutbox{path: tmpDir}
	err := ob.Append(model.InOutEvent{EventID: "evt"})
	if err == nil {
		t.Error("expected error appending to a directory path")
	}
}

func TestFileOutbox_ReplayDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	ob := &FileOutbox{path: tmpDir}
	_, err := ob.Replay(context.Background(), func(ctx context.Context, e model.InOutEvent) error {
		return nil
	})
	if err == nil {
		t.Error("expected error replaying from a directory path")
	}
}

func TestFileOutbox_ReplayBadJSONLines(t *testing.T) {
	tmpDir := t.TempDir()
	ob, _ := NewFileOutbox(tmpDir)
	if err := os.WriteFile(ob.path, []byte("bad-json-line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := ob.Replay(context.Background(), func(ctx context.Context, e model.InOutEvent) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 replayed, got %d", n)
	}
}

func TestFileOutbox_ReplayStillPendingRewritePath(t *testing.T) {
	tmpDir := t.TempDir()
	ob, _ := NewFileOutbox(tmpDir)
	ev1 := model.InOutEvent{EventID: "evt1"}
	ev2 := model.InOutEvent{EventID: "evt2"}
	_ = ob.Append(ev1)
	_ = ob.Append(ev2)

	n, err := ob.Replay(context.Background(), func(ctx context.Context, e model.InOutEvent) error {
		if e.EventID == "evt1" {
			return nil // succeeds
		}
		return errors.New("still fails")
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 replayed, got %d", n)
	}

	// Verify only evt2 remains in outbox
	data, err := os.ReadFile(ob.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytesContains(data, []byte("evt2")) || bytesContains(data, []byte("evt1")) {
		t.Errorf("unexpected outbox contents: %s", string(data))
	}
}

func TestFileOutbox_RewriteLockedFailedPath(t *testing.T) {
	ob := &FileOutbox{path: "/invalid/path/that/does/not/exist/outbox.jsonl"}
	err := ob.rewriteLocked([]model.InOutEvent{{EventID: "evt"}})
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Errorf("expected PathError, got %v", err)
	}
}

func bytesContains(b, sub []byte) bool {
	for i := 0; i <= len(b)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if b[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
