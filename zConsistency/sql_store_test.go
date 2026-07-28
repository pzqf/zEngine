package zConsistency

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

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
