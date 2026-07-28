package zConsistency

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestCommit_RetryAndInconsistent 验证 INF-4：提交阶段对失败参与者有界重试；瞬时失败能重试成功
// 并置 Committed；永久失败重试耗尽则置 Inconsistent（而非误标 Aborted），且不会因 ctx 相关
// 的 break bug 提前放弃剩余参与者。
func TestCommit_RetryAndInconsistent(t *testing.T) {
	var mu sync.Mutex
	attempts := map[string]int{}

	okPrepare := func(context.Context, uint64, string, interface{}) error { return nil }
	noopRollback := func(context.Context, uint64, string, interface{}) error { return nil }
	commitFn := func(_ context.Context, _ uint64, participant string, _ interface{}) error {
		mu.Lock()
		attempts[participant]++
		n := attempts[participant]
		mu.Unlock()
		switch participant {
		case "flaky": // 前两次失败，第三次成功
			if n < 3 {
				return errors.New("transient")
			}
			return nil
		case "bad": // 永久失败
			return errors.New("permanent")
		default:
			return nil
		}
	}

	tm := NewTransactionManager(okPrepare, commitFn, noopRollback)
	ctx := context.Background()

	// Case 1：flaky+good —— flaky 重试后成功 → 整体 Committed。
	tx1 := tm.Begin("coord", []string{"flaky", "good"}, time.Second, nil)
	if err := tm.Prepare(ctx, tx1.ID); err != nil {
		t.Fatalf("prepare1: %v", err)
	}
	if err := tm.Commit(ctx, tx1.ID); err != nil {
		t.Fatalf("commit1 should succeed after retry, got: %v", err)
	}
	if st, _ := tm.GetTransactionState(tx1.ID); st != TransactionStateCommitted {
		t.Fatalf("tx1 state = %s, want committed", st)
	}
	if attempts["flaky"] != 3 {
		t.Fatalf("flaky attempts = %d, want 3 (2 fail + 1 success)", attempts["flaky"])
	}

	// Case 2：good+bad —— bad 重试耗尽 → 整体 Inconsistent，且 good 仍被提交（未因 break 提前放弃）。
	mu.Lock()
	goodBefore := attempts["good"]
	mu.Unlock()
	tx2 := tm.Begin("coord", []string{"good", "bad"}, time.Second, nil)
	if err := tm.Prepare(ctx, tx2.ID); err != nil {
		t.Fatalf("prepare2: %v", err)
	}
	if err := tm.Commit(ctx, tx2.ID); err == nil {
		t.Fatal("commit2 should return error (inconsistent)")
	}
	if st, _ := tm.GetTransactionState(tx2.ID); st != TransactionStateInconsistent {
		t.Fatalf("tx2 state = %s, want inconsistent", st)
	}
	if attempts["bad"] != 3 {
		t.Fatalf("bad attempts = %d, want 3 (retry exhausted)", attempts["bad"])
	}
	mu.Lock()
	goodAfter := attempts["good"]
	mu.Unlock()
	if goodAfter != goodBefore+1 {
		t.Fatalf("good should still be committed once in tx2 (no early break): before=%d after=%d", goodBefore, goodAfter)
	}
}
