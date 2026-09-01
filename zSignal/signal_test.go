package zSignal

import (
	"context"
	"testing"
	"time"
)

func TestNotifyContextCanBeCanceledAndReleased(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, release := NotifyContext(parent)
	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("signal context did not follow parent cancellation")
	}
	release()
	release()
}
