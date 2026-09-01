package zConsistency

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

type executorResult int64

func (r executorResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r executorResult) RowsAffected() (int64, error) { return int64(r), nil }

type recordingSQLExecutor struct {
	execCalls int
}

func (e *recordingSQLExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	e.execCalls++
	return executorResult(1), nil
}

func (*recordingSQLExecutor) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

func (*recordingSQLExecutor) QueryRowContext(context.Context, string, ...any) *sql.Row {
	return nil
}

func TestSQLStoresCanBindCallerTransactionExecutor(t *testing.T) {
	executor := &recordingSQLExecutor{}
	outbox := NewSQLOutboxWithExecutor(executor)
	inbox := NewSQLInboxWithExecutor(executor, "zmmo_inbox_tx_test")

	ctx := context.Background()
	if err := outbox.Enqueue(ctx, OutboxMessage{
		RequestID:   91001,
		Topic:       "transaction-test",
		Payload:     []byte("payload"),
		CreatedAt:   time.Now(),
		NextRetryAt: time.Now(),
	}); err != nil {
		t.Fatalf("enqueue through transaction executor: %v", err)
	}
	result, err := inbox.Acquire(ctx, 91002)
	if err != nil {
		t.Fatalf("acquire through transaction executor: %v", err)
	}
	if result != InboxAccepted {
		t.Fatalf("unexpected inbox result: got %s want %s", result, InboxAccepted)
	}
	if executor.execCalls != 2 {
		t.Fatalf("unexpected executor calls: got %d want 2", executor.execCalls)
	}
}

func TestDatabaseAndTransactionSatisfySQLExecutor(t *testing.T) {
	var _ SQLExecutor = (*sql.DB)(nil)
	var _ SQLExecutor = (*sql.Tx)(nil)
}

func TestSQLExecutorTransactionJoinsBusinessAndOutboxE3(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	const (
		businessTable = "zmmo_con02_executor_business"
		outboxTable   = "zmmo_con02_executor_outbox"
	)
	for _, statement := range []string{
		"DROP TABLE IF EXISTS " + outboxTable,
		"DROP TABLE IF EXISTS " + businessTable,
		"CREATE TABLE " + businessTable + " (id BIGINT NOT NULL PRIMARY KEY, value BIGINT NOT NULL) ENGINE=InnoDB",
		"INSERT INTO " + businessTable + " (id, value) VALUES (1, 0)",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare executor transaction schema with %q: %v", statement, err)
		}
	}
	defer func() {
		_, _ = db.Exec("DROP TABLE IF EXISTS " + outboxTable)
		_, _ = db.Exec("DROP TABLE IF EXISTS " + businessTable)
	}()
	outbox := NewSQLOutbox(db, WithSQLOutboxTable(outboxTable))
	if err := outbox.EnsureOutboxSchemaContext(ctx); err != nil {
		t.Fatalf("ensure outbox schema: %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin rollback transaction: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE "+businessTable+" SET value=7 WHERE id=1"); err != nil {
		t.Fatalf("update rollback business row: %v", err)
	}
	if err := NewSQLOutboxWithExecutor(tx, WithSQLOutboxTable(outboxTable)).Enqueue(ctx, OutboxMessage{
		RequestID: 94001, Topic: "executor-rollback", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("enqueue rollback outbox: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback joined transaction: %v", err)
	}
	assertExecutorBusinessValue(t, db, businessTable, 0)
	if _, err := outbox.Get(ctx, 94001); !errors.Is(err, ErrConsistencyEntryNotFound) {
		t.Fatalf("rolled-back Outbox row survived: %v", err)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin commit transaction: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE "+businessTable+" SET value=7 WHERE id=1"); err != nil {
		t.Fatalf("update committed business row: %v", err)
	}
	if err := NewSQLOutboxWithExecutor(tx, WithSQLOutboxTable(outboxTable)).Enqueue(ctx, OutboxMessage{
		RequestID: 94002, Topic: "executor-commit", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("enqueue committed outbox: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit joined transaction: %v", err)
	}
	assertExecutorBusinessValue(t, db, businessTable, 7)
	message, err := NewSQLOutbox(db, WithSQLOutboxTable(outboxTable)).Get(ctx, 94002)
	if err != nil || message.State != OutboxStateEnqueued {
		t.Fatalf("committed Outbox row = (%+v, %v)", message, err)
	}
}

func assertExecutorBusinessValue(t *testing.T, db *sql.DB, table string, want int64) {
	t.Helper()
	var got int64
	if err := db.QueryRow("SELECT value FROM " + table + " WHERE id=1").Scan(&got); err != nil {
		t.Fatalf("query executor business value: %v", err)
	}
	if got != want {
		t.Fatalf("executor business value=%d, want %d", got, want)
	}
}
