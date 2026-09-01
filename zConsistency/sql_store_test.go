package zConsistency

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestSQLStoresV2_StateContractAcrossRestart(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	suffix := time.Now().UnixNano()
	outboxTable := fmt.Sprintf("test_outbox_v2_%d", suffix)
	inboxTable := fmt.Sprintf("test_inbox_v2_%d", suffix)
	defer db.Exec("DROP TABLE IF EXISTS " + outboxTable)
	defer db.Exec("DROP TABLE IF EXISTS " + inboxTable)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	outbox := NewSQLOutbox(db, WithSQLOutboxTable(outboxTable), WithSQLRetryBackoff(time.Millisecond))
	inbox := NewSQLInbox(db, inboxTable)
	if err := outbox.EnsureOutboxSchemaContext(ctx); err != nil {
		t.Fatalf("EnsureOutboxSchemaContext() error = %v", err)
	}
	if err := inbox.EnsureInboxSchemaContext(ctx); err != nil {
		t.Fatalf("EnsureInboxSchemaContext() error = %v", err)
	}

	const outboxID = uint64(300001)
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: outboxID, Topic: "v2", Payload: []byte("payload")}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	assertSQLV2OutboxState(t, ctx, outbox, outboxID, OutboxStateEnqueued)
	if err := outbox.MarkApplied(ctx, outboxID); !errors.Is(err, ErrInvalidConsistencyTransition) {
		t.Fatalf("MarkApplied(enqueued) error = %v, want ErrInvalidConsistencyTransition", err)
	}
	if err := outbox.MarkTransported(ctx, outboxID); err != nil {
		t.Fatalf("MarkTransported() error = %v", err)
	}
	assertSQLV2OutboxState(t, ctx, NewSQLOutbox(db, WithSQLOutboxTable(outboxTable)), outboxID, OutboxStateTransported)
	if err := outbox.MarkApplied(ctx, outboxID); err != nil {
		t.Fatalf("MarkApplied() error = %v", err)
	}
	assertSQLV2OutboxState(t, ctx, outbox, outboxID, OutboxStateApplied)
	if err := outbox.Complete(ctx, outboxID, OutboxStateApplied); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if _, err := outbox.Get(ctx, outboxID); !errors.Is(err, ErrConsistencyEntryNotFound) {
		t.Fatalf("Get() after Complete error = %v, want ErrConsistencyEntryNotFound", err)
	}

	const inboxID = uint64(400001)
	if result, err := inbox.Acquire(ctx, inboxID); err != nil || result != InboxAccepted {
		t.Fatalf("first Acquire() = (%v, %v), want (InboxAccepted, nil)", result, err)
	}
	if result, err := inbox.Acquire(ctx, inboxID); err != nil || result != InboxInProgress {
		t.Fatalf("second Acquire() = (%v, %v), want (InboxInProgress, nil)", result, err)
	}
	if err := inbox.MarkProcessed(ctx, inboxID); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}
	restartedInbox := NewSQLInbox(db, inboxTable)
	if result, err := restartedInbox.Acquire(ctx, inboxID); err != nil || result != InboxProcessed {
		t.Fatalf("Acquire() after restart = (%v, %v), want (InboxProcessed, nil)", result, err)
	}
	if err := restartedInbox.Abandon(ctx, inboxID); !errors.Is(err, ErrInvalidConsistencyTransition) {
		t.Fatalf("Abandon(processed) error = %v, want ErrInvalidConsistencyTransition", err)
	}

	const retryID = uint64(400002)
	if result, err := inbox.Acquire(ctx, retryID); err != nil || result != InboxAccepted {
		t.Fatalf("Acquire(retry) = (%v, %v)", result, err)
	}
	if err := inbox.Abandon(ctx, retryID); err != nil {
		t.Fatalf("Abandon(retry) error = %v", err)
	}
	if result, err := inbox.Acquire(ctx, retryID); err != nil || result != InboxAccepted {
		t.Fatalf("Acquire(after abandon) = (%v, %v), want (InboxAccepted, nil)", result, err)
	}
}

func TestSQLStoresV2_ConcurrentTransitionsHaveStableResults(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	suffix := time.Now().UnixNano()
	outboxTable := fmt.Sprintf("test_outbox_v2_concurrent_%d", suffix)
	inboxTable := fmt.Sprintf("test_inbox_v2_concurrent_%d", suffix)
	defer db.Exec("DROP TABLE IF EXISTS " + outboxTable)
	defer db.Exec("DROP TABLE IF EXISTS " + inboxTable)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	outbox := NewSQLOutbox(db, WithSQLOutboxTable(outboxTable), WithSQLRetryBackoff(time.Millisecond))
	inbox := NewSQLInbox(db, inboxTable)
	if err := outbox.EnsureOutboxSchemaContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := inbox.EnsureInboxSchemaContext(ctx); err != nil {
		t.Fatal(err)
	}

	const idempotentOutboxID = uint64(310001)
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: idempotentOutboxID}); err != nil {
		t.Fatal(err)
	}
	assertConcurrentErrors(t, 32, func() error {
		return outbox.MarkTransported(ctx, idempotentOutboxID)
	}, nil)
	assertConcurrentErrors(t, 32, func() error {
		return outbox.MarkApplied(ctx, idempotentOutboxID)
	}, nil)
	assertSQLV2OutboxState(t, ctx, outbox, idempotentOutboxID, OutboxStateApplied)

	const competingOutboxID = uint64(310002)
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: competingOutboxID}); err != nil {
		t.Fatal(err)
	}
	if err := outbox.MarkTransported(ctx, competingOutboxID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsByOperation := make(chan error, 32)
	var wg sync.WaitGroup
	for index := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if index == 0 {
				errorsByOperation <- outbox.MarkApplied(ctx, competingOutboxID)
				return
			}
			errorsByOperation <- outbox.RecordAttempt(ctx, competingOutboxID, errors.New("retry"))
		}()
	}
	close(start)
	wg.Wait()
	close(errorsByOperation)
	for err := range errorsByOperation {
		if err != nil && !errors.Is(err, ErrInvalidConsistencyTransition) {
			t.Fatalf("competing outbox transition error = %v, want nil or ErrInvalidConsistencyTransition", err)
		}
	}

	const inboxID = uint64(410001)
	results := make(chan InboxAcceptResult, 64)
	operationErrors := make(chan error, 64)
	start = make(chan struct{})
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := inbox.Acquire(ctx, inboxID)
			results <- result
			operationErrors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(operationErrors)
	accepted, inProgress := 0, 0
	for result := range results {
		switch result {
		case InboxAccepted:
			accepted++
		case InboxInProgress:
			inProgress++
		default:
			t.Fatalf("concurrent Acquire() result = %v", result)
		}
	}
	for err := range operationErrors {
		if err != nil {
			t.Fatalf("concurrent Acquire() error = %v", err)
		}
	}
	if accepted != 1 || inProgress != 63 {
		t.Fatalf("concurrent Acquire() states = accepted:%d in-progress:%d, want 1/63", accepted, inProgress)
	}

	const finalizationID = uint64(410002)
	if result, err := inbox.Acquire(ctx, finalizationID); err != nil || result != InboxAccepted {
		t.Fatalf("Acquire(finalization) = (%v, %v)", result, err)
	}
	start = make(chan struct{})
	finalizationErrors := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		finalizationErrors <- inbox.MarkProcessed(ctx, finalizationID)
	}()
	go func() {
		defer wg.Done()
		<-start
		finalizationErrors <- inbox.Abandon(ctx, finalizationID)
	}()
	close(start)
	wg.Wait()
	close(finalizationErrors)
	succeeded := 0
	for err := range finalizationErrors {
		if err == nil {
			succeeded++
			continue
		}
		if !errors.Is(err, ErrInvalidConsistencyTransition) && !errors.Is(err, ErrConsistencyEntryNotFound) {
			t.Fatalf("competing inbox finalization error = %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("competing inbox finalizations succeeded = %d, want 1", succeeded)
	}
}

func assertConcurrentErrors(t *testing.T, count int, operation func() error, want error) {
	t.Helper()
	start := make(chan struct{})
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- operation()
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, want) {
			t.Fatalf("concurrent operation error = %v, want %v", err, want)
		}
	}
}

func assertSQLV2OutboxState(t *testing.T, ctx context.Context, outbox *SQLOutbox, requestID uint64, want OutboxState) {
	t.Helper()
	message, err := outbox.Get(ctx, requestID)
	if err != nil {
		t.Fatalf("Get(%d) error = %v", requestID, err)
	}
	if message.State != want {
		t.Fatalf("Get(%d).State = %v, want %v", requestID, message.State, want)
	}
}

// openTestDB 打开测试用 MySQL。未设置 ZMMO_TEST_MYSQL_DSN 则跳过（CI 无需 DB）。
// DSN 示例：root:123456@tcp(192.168.251.134:3306)/global?parseTime=true&loc=Local
func openTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("ZMMO_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("ZMMO_TEST_MYSQL_DSN not set; skipping SQL Outbox/Inbox integration test")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("cannot reach MySQL (%v); skipping", err)
	}
	return db
}

// TestSQLOutbox_PersistAcrossRestart 验证 Outbox 落库、可续投、且跨"重启"（新实例）存活。
func TestSQLOutbox_PersistAcrossRestart(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	const table = "test_outbox_store"
	_, _ = db.Exec("DROP TABLE IF EXISTS " + table)
	ob := NewSQLOutbox(db, WithSQLOutboxTable(table), WithSQLMaxRetries(3), WithSQLRetryBackoff(10*time.Millisecond))
	if err := ob.EnsureOutboxSchema(); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	defer db.Exec("DROP TABLE IF EXISTS " + table)

	const id = uint64(100001)
	ob.Add(OutboxMessage{RequestID: id, Topic: "combat", TargetServerID: "map-301", TargetMapID: 1001, ProtoID: 406, Payload: []byte("payload-bytes")})

	if got := ob.CountPending(); got != 1 {
		t.Fatalf("pending after add: got %d want 1", got)
	}
	retry := ob.ListRetryable(time.Now(), 10)
	if len(retry) != 1 || retry[0].RequestID != id || string(retry[0].Payload) != "payload-bytes" {
		t.Fatalf("retryable mismatch: %+v", retry)
	}

	// 模拟进程重启：全新 store 实例读同一张表，应仍看到未投递消息（持久性）。
	ob2 := NewSQLOutbox(db, WithSQLOutboxTable(table))
	if got := ob2.CountPending(); got != 1 {
		t.Fatalf("pending must survive restart: got %d want 1", got)
	}

	// 记一次失败尝试 → 仍待投递(sent=0)，但退避未到则暂不可 retry。
	ob.MarkAttempt(id, errBoom{})
	if got := ob.CountPending(); got != 1 {
		t.Fatalf("pending after failed attempt: got %d want 1", got)
	}
	if got := ob.ListRetryable(time.Now(), 10); len(got) != 0 {
		t.Fatalf("should not be retryable before backoff elapses, got %d", len(got))
	}
	time.Sleep(30 * time.Millisecond) // 超过 10ms 退避
	if got := ob.ListRetryable(time.Now(), 10); len(got) != 1 {
		t.Fatalf("should be retryable after backoff, got %d", len(got))
	}

	// 死信后不再出现在 pending/retryable。
	ob.MarkDeadLetter(id, "exceeded")
	if got := ob.CountDeadLetters(); got != 1 {
		t.Fatalf("dead letters: got %d want 1", got)
	}
	if got := ob.CountPending(); got != 0 {
		t.Fatalf("dead letter should not count as pending, got %d", got)
	}

	// MarkSent 直接删除条目（GS-1：fire-and-forget 无 ack，已发送即终态，避免 outbox 无界增长）。
	const id2 = uint64(100002)
	ob.Add(OutboxMessage{RequestID: id2, Topic: "t", ProtoID: 1, Payload: []byte("x")})
	if got := ob.CountPending(); got != 1 {
		t.Fatalf("pending after add id2: got %d want 1", got)
	}
	ob.MarkSent(id2)
	if got := ob.CountPending(); got != 0 {
		t.Fatalf("MarkSent must remove the row (pending): got %d want 0", got)
	}
}

// TestSQLInbox_IdempotentAcrossRestart 验证 Inbox 去重且跨"重启"保持幂等。
func TestSQLInbox_IdempotentAcrossRestart(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	const table = "test_inbox_store"
	_, _ = db.Exec("DROP TABLE IF EXISTS " + table)
	in := NewSQLInbox(db, table)
	if err := in.EnsureInboxSchema(); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	defer db.Exec("DROP TABLE IF EXISTS " + table)

	const id = uint64(200002)
	if !in.TryAccept(id) {
		t.Fatalf("first TryAccept must succeed")
	}
	if in.TryAccept(id) {
		t.Fatalf("second TryAccept must be false (duplicate rejected)")
	}
	if !in.IsProcessed(id) {
		t.Fatalf("IsProcessed must be true after accept")
	}

	// 模拟重启：新 Inbox 实例仍应识别该请求已处理（去重跨重启存活）。
	in2 := NewSQLInbox(db, table)
	if in2.TryAccept(id) {
		t.Fatalf("dedup must survive restart: TryAccept must stay false")
	}

	// request_id==0 恒接受（无幂等键）。
	if !in.TryAccept(0) {
		t.Fatalf("request_id 0 must always be accepted")
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
