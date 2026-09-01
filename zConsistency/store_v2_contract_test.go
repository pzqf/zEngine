package zConsistency

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	_ OutboxStoreV2 = (*MemoryOutbox)(nil)
	_ OutboxStoreV2 = (*SQLOutbox)(nil)
	_ InboxStoreV2  = (*MemoryInbox)(nil)
	_ InboxStoreV2  = (*SQLInbox)(nil)
)

func TestMemoryOutboxV2StatesAndExplicitCompletion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	outbox := NewMemoryOutbox()
	payload := []byte("payload")
	message := OutboxMessage{RequestID: 101, Topic: "grant", Payload: payload}
	if err := outbox.Enqueue(ctx, message); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	payload[0] = 'X'

	assertOutboxState(t, outbox, 101, OutboxStateEnqueued)
	stored, err := outbox.Get(ctx, 101)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got := string(stored.Payload); got != "payload" {
		t.Fatalf("stored payload = %q, want defensive copy", got)
	}
	stored.Payload[0] = 'Y'
	storedAgain, err := outbox.Get(ctx, 101)
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if got := string(storedAgain.Payload); got != "payload" {
		t.Fatalf("Get() leaked payload ownership: %q", got)
	}

	if err := outbox.MarkTransported(ctx, 101); err != nil {
		t.Fatalf("MarkTransported() error = %v", err)
	}
	assertOutboxState(t, outbox, 101, OutboxStateTransported)

	if err := outbox.MarkApplied(ctx, 101); err != nil {
		t.Fatalf("MarkApplied() error = %v", err)
	}
	assertOutboxState(t, outbox, 101, OutboxStateApplied)

	if err := outbox.Complete(ctx, 101, OutboxStateApplied); err != nil {
		t.Fatalf("Complete(applied) error = %v", err)
	}
	if _, err := outbox.Get(ctx, 101); !errors.Is(err, ErrConsistencyEntryNotFound) {
		t.Fatalf("Get() after completion error = %v, want ErrConsistencyEntryNotFound", err)
	}

	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: 102, Topic: "transport-only"}); err != nil {
		t.Fatalf("Enqueue(transport-only) error = %v", err)
	}
	if err := outbox.MarkTransported(ctx, 102); err != nil {
		t.Fatalf("MarkTransported(transport-only) error = %v", err)
	}
	if err := outbox.Complete(ctx, 102, OutboxStateTransported); err != nil {
		t.Fatalf("Complete(transport-only) error = %v", err)
	}
	if _, err := outbox.Get(ctx, 102); !errors.Is(err, ErrConsistencyEntryNotFound) {
		t.Fatalf("transport-only record retained: %v", err)
	}
}

func TestMemoryOutboxV2RejectsInvalidOperations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	outbox := NewMemoryOutbox()
	if err := outbox.Enqueue(ctx, OutboxMessage{}); !errors.Is(err, ErrInvalidConsistencyRequestID) {
		t.Fatalf("Enqueue(zero id) error = %v, want ErrInvalidConsistencyRequestID", err)
	}
	if err := outbox.MarkTransported(ctx, 999); !errors.Is(err, ErrConsistencyEntryNotFound) {
		t.Fatalf("MarkTransported(missing) error = %v, want ErrConsistencyEntryNotFound", err)
	}
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: 201}); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: 201}); !errors.Is(err, ErrConsistencyEntryExists) {
		t.Fatalf("duplicate Enqueue() error = %v, want ErrConsistencyEntryExists", err)
	}
	if err := outbox.MarkApplied(ctx, 201); !errors.Is(err, ErrInvalidConsistencyTransition) {
		t.Fatalf("MarkApplied(enqueued) error = %v, want ErrInvalidConsistencyTransition", err)
	}
	if err := outbox.Complete(ctx, 201, OutboxStateEnqueued); !errors.Is(err, ErrInvalidConsistencyTransition) {
		t.Fatalf("Complete(enqueued) error = %v, want ErrInvalidConsistencyTransition", err)
	}
	if err := outbox.MoveToDeadLetter(ctx, 201, "exhausted"); err != nil {
		t.Fatalf("MoveToDeadLetter() error = %v", err)
	}
	assertOutboxState(t, outbox, 201, OutboxStateDeadLetter)
	if err := outbox.MarkApplied(ctx, 201); !errors.Is(err, ErrInvalidConsistencyTransition) {
		t.Fatalf("MarkApplied(dead-letter) error = %v, want ErrInvalidConsistencyTransition", err)
	}
}

func TestMemoryOutboxV2ListsAndCountsReturnErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	outbox := NewMemoryOutbox(WithRetryBackoff(time.Nanosecond))
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: 301}); err != nil {
		t.Fatal(err)
	}
	if err := outbox.RecordAttempt(ctx, 301, errors.New("transport failed")); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: 302}); err != nil {
		t.Fatal(err)
	}
	if err := outbox.MarkTransported(ctx, 302); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: 303}); err != nil {
		t.Fatal(err)
	}
	if err := outbox.MoveToDeadLetter(ctx, 303, "exhausted"); err != nil {
		t.Fatal(err)
	}

	unresolved, err := outbox.ListUnresolved(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnresolved() error = %v", err)
	}
	if len(unresolved) != 2 {
		t.Fatalf("ListUnresolved() len = %d, want 2", len(unresolved))
	}
	count, err := outbox.CountUnresolved(ctx)
	if err != nil || count != 2 {
		t.Fatalf("CountUnresolved() = (%d, %v), want (2, nil)", count, err)
	}
	retryable, err := outbox.ListRetryableContext(ctx, time.Now().Add(time.Second), 10)
	if err != nil {
		t.Fatalf("ListRetryableContext() error = %v", err)
	}
	if len(retryable) != 2 {
		t.Fatalf("ListRetryableContext() len = %d, want 2", len(retryable))
	}
	dead, err := outbox.ListDeadLettersContext(ctx, 10)
	if err != nil || len(dead) != 1 {
		t.Fatalf("ListDeadLettersContext() = (%d, %v), want (1, nil)", len(dead), err)
	}
}

func TestMemoryInboxV2DistinguishesAdmissionStates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inbox := NewMemoryInbox()
	const requestID = uint64(401)

	var accepted atomic.Int32
	var inProgress atomic.Int32
	var processed atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := inbox.Acquire(ctx, requestID)
			if err != nil {
				t.Errorf("Acquire() error = %v", err)
				return
			}
			switch result {
			case InboxAccepted:
				accepted.Add(1)
			case InboxInProgress:
				inProgress.Add(1)
			case InboxProcessed:
				processed.Add(1)
			default:
				t.Errorf("Acquire() result = %v", result)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("accepted = %d, want 1", got)
	}
	if got := inProgress.Load(); got != 63 {
		t.Fatalf("in progress = %d, want 63", got)
	}
	if got := processed.Load(); got != 0 {
		t.Fatalf("processed before completion = %d, want 0", got)
	}

	if err := inbox.MarkProcessed(ctx, requestID); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}
	for range 8 {
		result, err := inbox.Acquire(ctx, requestID)
		if err != nil || result != InboxProcessed {
			t.Fatalf("Acquire() after completion = (%v, %v), want (InboxProcessed, nil)", result, err)
		}
	}
	if err := inbox.Abandon(ctx, requestID); !errors.Is(err, ErrInvalidConsistencyTransition) {
		t.Fatalf("Abandon(processed) error = %v, want ErrInvalidConsistencyTransition", err)
	}

	const retryID = uint64(402)
	if result, err := inbox.Acquire(ctx, retryID); err != nil || result != InboxAccepted {
		t.Fatalf("Acquire(retry) = (%v, %v)", result, err)
	}
	if err := inbox.Abandon(ctx, retryID); err != nil {
		t.Fatalf("Abandon() error = %v", err)
	}
	if result, err := inbox.Acquire(ctx, retryID); err != nil || result != InboxAccepted {
		t.Fatalf("Acquire(after abandon) = (%v, %v), want (InboxAccepted, nil)", result, err)
	}
}

func TestV2StoresHonorCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewMemoryOutbox().Enqueue(ctx, OutboxMessage{RequestID: 501}); !errors.Is(err, context.Canceled) {
		t.Fatalf("MemoryOutbox.Enqueue() error = %v, want context.Canceled", err)
	}
	if result, err := NewMemoryInbox().Acquire(ctx, 501); !errors.Is(err, context.Canceled) || result == InboxAccepted {
		t.Fatalf("MemoryInbox.Acquire() = (%v, %v), want non-accepted context.Canceled", result, err)
	}

	db := openFailingConsistencyDB(t)
	if err := NewSQLOutbox(db).Enqueue(ctx, OutboxMessage{RequestID: 502}); !errors.Is(err, context.Canceled) {
		t.Fatalf("SQLOutbox.Enqueue() error = %v, want context.Canceled", err)
	}
	if result, err := NewSQLInbox(db, "inbox_entries").Acquire(ctx, 502); !errors.Is(err, context.Canceled) || result == InboxAccepted {
		t.Fatalf("SQLInbox.Acquire() = (%v, %v), want non-accepted context.Canceled", result, err)
	}
}

func TestSQLV2PropagatesStoreErrorsAndInboxFailsClosed(t *testing.T) {
	t.Parallel()

	db := openFailingConsistencyDB(t)
	ctx := context.Background()
	outbox := NewSQLOutbox(db)
	if err := outbox.EnsureOutboxSchemaContext(ctx); !errors.Is(err, errForcedConsistencyStore) {
		t.Fatalf("EnsureOutboxSchemaContext() error = %v, want forced store error", err)
	}
	if err := outbox.Enqueue(ctx, OutboxMessage{RequestID: 601}); !errors.Is(err, errForcedConsistencyStore) {
		t.Fatalf("SQLOutbox.Enqueue() error = %v, want forced store error", err)
	}
	outboxChecks := []struct {
		name string
		call func() error
	}{
		{"Get", func() error { _, err := outbox.Get(ctx, 601); return err }},
		{"MarkTransported", func() error { return outbox.MarkTransported(ctx, 601) }},
		{"MarkApplied", func() error { return outbox.MarkApplied(ctx, 601) }},
		{"RecordAttempt", func() error { return outbox.RecordAttempt(ctx, 601, errors.New("send")) }},
		{"MoveToDeadLetter", func() error { return outbox.MoveToDeadLetter(ctx, 601, "dead") }},
		{"Complete", func() error { return outbox.Complete(ctx, 601, OutboxStateTransported) }},
		{"ListUnresolved", func() error { _, err := outbox.ListUnresolved(ctx, 10); return err }},
		{"ListRetryableContext", func() error { _, err := outbox.ListRetryableContext(ctx, time.Now(), 10); return err }},
		{"ListDeadLettersContext", func() error { _, err := outbox.ListDeadLettersContext(ctx, 10); return err }},
		{"CountUnresolved", func() error { _, err := outbox.CountUnresolved(ctx); return err }},
		{"CountRetryableContext", func() error { _, err := outbox.CountRetryableContext(ctx, time.Now()); return err }},
		{"CountDeadLettersContext", func() error { _, err := outbox.CountDeadLettersContext(ctx); return err }},
		{"PurgeDeadLettersContext", func() error { _, err := outbox.PurgeDeadLettersContext(ctx, 0); return err }},
	}
	for _, check := range outboxChecks {
		if err := check.call(); !errors.Is(err, errForcedConsistencyStore) {
			t.Errorf("%s() error = %v, want forced store error", check.name, err)
		}
	}

	inbox := NewSQLInbox(db, "inbox_entries")
	if err := inbox.EnsureInboxSchemaContext(ctx); !errors.Is(err, errForcedConsistencyStore) {
		t.Fatalf("EnsureInboxSchemaContext() error = %v, want forced store error", err)
	}
	result, err := inbox.Acquire(ctx, 601)
	if !errors.Is(err, errForcedConsistencyStore) {
		t.Fatalf("SQLInbox.Acquire() error = %v, want forced store error", err)
	}
	if result == InboxAccepted {
		t.Fatal("SQLInbox.Acquire() accepted work after durable-store failure")
	}
	inboxChecks := []struct {
		name string
		call func() error
	}{
		{"Status", func() error { _, err := inbox.Status(ctx, 601); return err }},
		{"MarkProcessed", func() error { return inbox.MarkProcessed(ctx, 601) }},
		{"Abandon", func() error { return inbox.Abandon(ctx, 601) }},
		{"CleanupContext", func() error { _, err := inbox.CleanupContext(ctx, 0); return err }},
	}
	for _, check := range inboxChecks {
		if err := check.call(); !errors.Is(err, errForcedConsistencyStore) {
			t.Errorf("%s() error = %v, want forced store error", check.name, err)
		}
	}
}

func TestLegacyMemoryAPIsRetainSourceBehavior(t *testing.T) {
	t.Parallel()

	var oldOutbox OutboxStore = NewMemoryOutbox()
	oldOutbox.Add(OutboxMessage{RequestID: 701, Payload: []byte("legacy")})
	if got := oldOutbox.CountPending(); got != 1 {
		t.Fatalf("legacy CountPending() = %d, want 1", got)
	}
	oldOutbox.MarkSent(701)
	if got := oldOutbox.CountPending(); got != 0 {
		t.Fatalf("legacy MarkSent() did not remove transport-only record: %d", got)
	}

	var oldInbox InboxStore = NewMemoryInbox()
	if !oldInbox.TryAccept(702) || oldInbox.TryAccept(702) {
		t.Fatal("legacy TryAccept() bool semantics changed")
	}
	oldInbox.Release(702)
	if !oldInbox.TryAccept(702) {
		t.Fatal("legacy Release() no longer allows retry")
	}
}

func TestMemoryLegacyAndV2APIsShareTransitionSerialization(t *testing.T) {
	t.Parallel()

	outbox := NewMemoryOutbox()
	outbox.v2Mu.Lock()
	outboxDone := make(chan struct{})
	go func() {
		outbox.Add(OutboxMessage{RequestID: 711, Payload: []byte("legacy")})
		close(outboxDone)
	}()
	assertBlockedWhileTransitionLocked(t, outboxDone, "legacy outbox Add")
	outbox.v2Mu.Unlock()
	<-outboxDone

	inbox := NewMemoryInbox()
	inbox.v2Mu.Lock()
	inboxDone := make(chan struct{})
	go func() {
		inbox.TryAccept(712)
		close(inboxDone)
	}()
	assertBlockedWhileTransitionLocked(t, inboxDone, "legacy inbox TryAccept")
	inbox.v2Mu.Unlock()
	<-inboxDone
}

func TestMemoryLegacyOutboxDoesNotLeakPayloadOwnershipIntoV2(t *testing.T) {
	t.Parallel()

	outbox := NewMemoryOutbox()
	payload := []byte("legacy")
	outbox.Add(OutboxMessage{RequestID: 713, Payload: payload})
	payload[0] = 'X'

	stored, err := outbox.Get(context.Background(), 713)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got := string(stored.Payload); got != "legacy" {
		t.Fatalf("legacy Add leaked input payload ownership: %q", got)
	}

	listed := outbox.ListPending(1)
	if len(listed) != 1 {
		t.Fatalf("ListPending() len = %d, want 1", len(listed))
	}
	listed[0].Payload[0] = 'Y'
	stored, err = outbox.Get(context.Background(), 713)
	if err != nil {
		t.Fatalf("Get() after ListPending error = %v", err)
	}
	if got := string(stored.Payload); got != "legacy" {
		t.Fatalf("legacy ListPending leaked stored payload ownership: %q", got)
	}
}

func assertBlockedWhileTransitionLocked(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-done:
		t.Fatalf("%s bypassed the V2 transition lock", operation)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertOutboxState(t *testing.T, outbox OutboxStoreV2, requestID uint64, want OutboxState) {
	t.Helper()
	message, err := outbox.Get(context.Background(), requestID)
	if err != nil {
		t.Fatalf("Get(%d) error = %v", requestID, err)
	}
	if message.State != want {
		t.Fatalf("Get(%d).State = %v, want %v", requestID, message.State, want)
	}
}

var errForcedConsistencyStore = errors.New("forced consistency store failure")

const failingConsistencyDriverName = "zconsistency_contract_failure"

var registerFailingConsistencyDriver sync.Once

func openFailingConsistencyDB(t *testing.T) *sql.DB {
	t.Helper()
	registerFailingConsistencyDriver.Do(func() {
		sql.Register(failingConsistencyDriverName, failingConsistencyDriver{})
	})
	db, err := sql.Open(failingConsistencyDriverName, "")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type failingConsistencyDriver struct{}

func (failingConsistencyDriver) Open(string) (driver.Conn, error) {
	return failingConsistencyConn{}, nil
}

type failingConsistencyConn struct{}

func (failingConsistencyConn) Prepare(string) (driver.Stmt, error) {
	return nil, errForcedConsistencyStore
}

func (failingConsistencyConn) Close() error { return nil }

func (failingConsistencyConn) Begin() (driver.Tx, error) {
	return nil, errForcedConsistencyStore
}

func (failingConsistencyConn) ExecContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errForcedConsistencyStore
}

func (failingConsistencyConn) QueryContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errForcedConsistencyStore
}

func (failingConsistencyConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (failingConsistencyConn) String() string { return fmt.Sprint(errForcedConsistencyStore) }
