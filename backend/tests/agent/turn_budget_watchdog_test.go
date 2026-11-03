package agent_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
)

func TestStreamWatchdogHealthyStream(t *testing.T) {
	ctx := context.Background()
	watchdog := application.NewStreamWatchdog(100 * time.Millisecond)

	inputChan := make(chan application.StreamChunk, 4)
	go func() {
		defer close(inputChan)
		inputChan <- application.StreamChunk{Type: "thinking", ThinkingText: "思考中..."}
		time.Sleep(30 * time.Millisecond) // within 100ms idle timeout
		inputChan <- application.StreamChunk{Type: "text", DeltaText: "你好"}
		time.Sleep(30 * time.Millisecond)
		inputChan <- application.StreamChunk{Type: "text", DeltaText: "世界"}
	}()

	watchedChan := watchdog.Watch(ctx, inputChan)

	chunks := make([]application.StreamChunk, 0)
	for chunk := range watchedChan {
		if chunk.Error != nil {
			t.Fatalf("unexpected chunk error: %v", chunk.Error)
		}
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
}

func TestStreamWatchdogStalledStream(t *testing.T) {
	ctx := context.Background()
	// Short watchdog idle timeout of 50ms for test
	watchdog := application.NewStreamWatchdog(50 * time.Millisecond)

	inputChan := make(chan application.StreamChunk)
	go func() {
		// Send one chunk, then freeze/stall upstream
		inputChan <- application.StreamChunk{Type: "text", DeltaText: "第一块"}
		time.Sleep(200 * time.Millisecond) // stall > 50ms
		inputChan <- application.StreamChunk{Type: "text", DeltaText: "迟到的第二块"}
		close(inputChan)
	}()

	watchedChan := watchdog.Watch(ctx, inputChan)

	receivedFirst := false
	gotStalledErr := false

	for chunk := range watchedChan {
		if chunk.Error != nil {
			if errors.Is(chunk.Error, application.ErrStreamStalled) {
				gotStalledErr = true
			}
			break
		}
		if chunk.DeltaText == "第一块" {
			receivedFirst = true
		}
	}

	if !receivedFirst {
		t.Errorf("expected to receive first chunk before stall")
	}
	if !gotStalledErr {
		t.Errorf("expected ErrStreamStalled when stream hung for 200ms")
	}
}

func TestTurnBudgetTrackerEnforcement(t *testing.T) {
	t.Run("ModelStepCeiling40", func(t *testing.T) {
		tracker := application.NewTurnBudgetTracker(40, 40, 100000000)

		// 40 steps must succeed
		for i := 0; i < 40; i++ {
			if err := tracker.RecordModelStep(100); err != nil {
				t.Fatalf("step %d failed: %v", i, err)
			}
		}

		// Step 41 must fail with ErrTurnBudgetExceeded
		err := tracker.RecordModelStep(100)
		if !errors.Is(err, application.ErrTurnBudgetExceeded) {
			t.Fatalf("expected ErrTurnBudgetExceeded at step 41, got %v", err)
		}
	})

	t.Run("ToolStepCeiling40", func(t *testing.T) {
		tracker := application.NewTurnBudgetTracker(40, 40, 100000000)

		for i := 0; i < 40; i++ {
			if err := tracker.RecordToolStep(50); err != nil {
				t.Fatalf("tool step %d failed: %v", i, err)
			}
		}

		err := tracker.RecordToolStep(50)
		if !errors.Is(err, application.ErrTurnBudgetExceeded) {
			t.Fatalf("expected ErrTurnBudgetExceeded for tool step 41, got %v", err)
		}
	})

	t.Run("BudgetOverdraft", func(t *testing.T) {
		tracker := application.NewTurnBudgetTracker(40, 40, 5000) // 5000 micros reservation

		// 4000 micros spent -> OK
		if err := tracker.RecordModelStep(4000); err != nil {
			t.Fatalf("first spend failed: %v", err)
		}

		// Another 2000 micros spent -> total 6000 > 5000 -> ErrBudgetOverdraft
		err := tracker.RecordModelStep(2000)
		if !errors.Is(err, application.ErrBudgetOverdraft) {
			t.Fatalf("expected ErrBudgetOverdraft, got %v", err)
		}
	})
}
