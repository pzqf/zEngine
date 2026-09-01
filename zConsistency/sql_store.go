package zConsistency

import (
	"context"
	"database/sql"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"go.uber.org/zap"
)

// SQL 持久化的 Outbox/Inbox 实现（成熟化改造 Phase 3.4）。
//
// 目的：进程重启不丢在途跨服消息——Outbox 未确认的消息、Inbox 的去重记录都落 MySQL，
// 重启后经 ListRetryable 续投、经 TryAccept 保持幂等。与内存实现 MemoryOutbox/MemoryInbox
// 共用 OutboxStore/InboxStore 接口，是 drop-in 替换（构造点按配置二选一）。
//
// 设计取舍：
//   - 接受标准 *sql.DB（driver-agnostic），本包不 import 具体驱动，由调用方注册（mysql/…）。
//   - 接口方法不返回 error（与内存版一致，调用方按 fire-and-forget 使用），故 SQL 错误在内部
//     经 zLog 记录而非上抛。
//   - DDL 为 MySQL 方言；表名可配置以便多实例/测试隔离。

const (
	defaultOutboxTable = "outbox_messages"
	defaultInboxTable  = "inbox_entries"
	sqlOpTimeout       = 5 * time.Second
)

// 编译期确认 SQL 实现满足与内存版相同的接口（drop-in 替换）。
var (
	_ OutboxStore   = (*SQLOutbox)(nil)
	_ InboxStore    = (*SQLInbox)(nil)
	_ OutboxStoreV2 = (*SQLOutbox)(nil)
	_ InboxStoreV2  = (*SQLInbox)(nil)
)

// ---------------- SQLOutbox ----------------

type SQLOutbox struct {
	db            *sql.DB
	table         string
	maxRetries    int
	retryBackoff  time.Duration
	maxRetryDelay time.Duration
}

type SQLOutboxOption func(*SQLOutbox)

func WithSQLOutboxTable(name string) SQLOutboxOption {
	return func(o *SQLOutbox) {
		if name != "" {
			o.table = name
		}
	}
}

func WithSQLMaxRetries(n int) SQLOutboxOption {
	return func(o *SQLOutbox) { o.maxRetries = n }
}

func WithSQLRetryBackoff(d time.Duration) SQLOutboxOption {
	return func(o *SQLOutbox) { o.retryBackoff = d }
}

func WithSQLMaxRetryDelay(d time.Duration) SQLOutboxOption {
	return func(o *SQLOutbox) { o.maxRetryDelay = d }
}

// NewSQLOutbox 创建 SQL 持久化 Outbox。db 需已连接；建表用 EnsureOutboxSchema。
func NewSQLOutbox(db *sql.DB, opts ...SQLOutboxOption) *SQLOutbox {
	o := &SQLOutbox{
		db:            db,
		table:         defaultOutboxTable,
		maxRetries:    5,
		retryBackoff:  500 * time.Millisecond,
		maxRetryDelay: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// EnsureOutboxSchema 幂等建表（CREATE TABLE IF NOT EXISTS）。
func (o *SQLOutbox) EnsureOutboxSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	ddl := "CREATE TABLE IF NOT EXISTS " + o.table + ` (
		request_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
		topic VARCHAR(191) NOT NULL DEFAULT '',
		target_server_id VARCHAR(64) NOT NULL DEFAULT '',
		target_map_id INT NOT NULL DEFAULT 0,
		proto_id INT NOT NULL DEFAULT 0,
		payload LONGBLOB,
		sent TINYINT NOT NULL DEFAULT 0,
		acked TINYINT NOT NULL DEFAULT 0,
		attempts INT NOT NULL DEFAULT 0,
		last_error TEXT,
		dead_letter TINYINT NOT NULL DEFAULT 0,
		created_at DATETIME(3) NOT NULL,
		last_attempt_at DATETIME(3) NULL,
		next_retry_at DATETIME(3) NULL,
		KEY idx_outbox_pending (sent, dead_letter, next_retry_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	_, err := o.db.ExecContext(ctx, ddl)
	return err
}

func (o *SQLOutbox) exec(query string, args ...interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	if _, err := o.db.ExecContext(ctx, query, args...); err != nil {
		zLog.Error("SQLOutbox exec failed", zap.String("query", query), zap.Error(err))
	}
}

func (o *SQLOutbox) Add(msg OutboxMessage) {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.NextRetryAt.IsZero() {
		msg.NextRetryAt = msg.CreatedAt
	}
	o.exec("INSERT INTO "+o.table+` (request_id, topic, target_server_id, target_map_id, proto_id, payload,
		sent, acked, attempts, last_error, dead_letter, created_at, last_attempt_at, next_retry_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE topic=VALUES(topic), target_server_id=VALUES(target_server_id),
		target_map_id=VALUES(target_map_id), proto_id=VALUES(proto_id), payload=VALUES(payload),
		sent=VALUES(sent), acked=VALUES(acked), attempts=VALUES(attempts), dead_letter=VALUES(dead_letter),
		created_at=VALUES(created_at), next_retry_at=VALUES(next_retry_at)`,
		msg.RequestID, msg.Topic, msg.TargetServerID, msg.TargetMapID, msg.ProtoID, msg.Payload,
		boolToInt(msg.Sent), boolToInt(msg.Acked), msg.Attempts, msg.LastError, boolToInt(msg.DeadLetter),
		msg.CreatedAt, nullTime(msg.LastAttemptAt), nullTime(msg.NextRetryAt))
}

// MarkSent 标记已发送。GS-1：直接删除行（与内存版一致）——fire-and-forget 无 ack，"已发送"即终态，
// 保留会让 outbox 表随每次跨服发送无界增长。发送失败走 MarkAttempt（保留待重试）。
func (o *SQLOutbox) MarkSent(requestID uint64) {
	o.exec("DELETE FROM "+o.table+" WHERE request_id=?", requestID)
}

func (o *SQLOutbox) MarkAcked(requestID uint64) {
	o.exec("DELETE FROM "+o.table+" WHERE request_id=?", requestID)
}

func (o *SQLOutbox) MarkAttempt(requestID uint64, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()

	// INF-10: 计数递增用 SQL 原子表达式 `attempts=attempts+1`，避免"SELECT→Go 自增→UPDATE 写回"
	// 的读改写在并发下丢更新（两个并发 MarkAttempt 读到同值、都写 old+1→只加了一次）。
	// 仅为计算指数退避时长读一次旧 attempts（近似即可，退避早一档无害）。
	var attempts int
	if err := o.db.QueryRowContext(ctx, "SELECT attempts FROM "+o.table+" WHERE request_id=?", requestID).Scan(&attempts); err != nil {
		if err != sql.ErrNoRows {
			zLog.Error("SQLOutbox MarkAttempt read failed", zap.Uint64("request_id", requestID), zap.Error(err))
		}
		return
	}

	if cause == nil {
		o.exec("UPDATE "+o.table+" SET attempts=attempts+1, last_attempt_at=? WHERE request_id=?",
			time.Now(), requestID)
		return
	}

	delay := o.retryBackoff * time.Duration(1<<uint(attempts))
	if delay > o.maxRetryDelay {
		delay = o.maxRetryDelay
	}
	o.exec("UPDATE "+o.table+" SET attempts=attempts+1, last_attempt_at=?, last_error=?, sent=0, next_retry_at=? WHERE request_id=?",
		time.Now(), cause.Error(), time.Now().Add(delay), requestID)
}

func (o *SQLOutbox) MarkDeadLetter(requestID uint64, reason string) {
	o.exec("UPDATE "+o.table+" SET dead_letter=1, last_error=?, last_attempt_at=? WHERE request_id=?",
		reason, time.Now(), requestID)
	zLog.Error("Outbox message moved to dead-letter (SQL)", zap.Uint64("request_id", requestID), zap.String("reason", reason))
}

func (o *SQLOutbox) query(where, orderBy string, limit int, args ...interface{}) []OutboxMessage {
	if limit <= 0 {
		limit = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	// INF-10: 必须带 ORDER BY，否则在 LIMIT 下返回顺序不定→部分到期消息可能长期被截断在窗口外
	// 而饿死（永不重投）。按到期时间/创建时间升序取，保证最该处理的先出。
	q := "SELECT request_id, topic, target_server_id, target_map_id, proto_id, payload, sent, acked, attempts, " +
		"last_error, dead_letter, created_at, last_attempt_at, next_retry_at FROM " + o.table + " WHERE " + where + " " + orderBy + " LIMIT ?"
	args = append(args, limit)
	rows, err := o.db.QueryContext(ctx, q, args...)
	if err != nil {
		zLog.Error("SQLOutbox query failed", zap.String("where", where), zap.Error(err))
		return nil
	}
	defer rows.Close()

	out := make([]OutboxMessage, 0, limit)
	for rows.Next() {
		var m OutboxMessage
		var sentI, ackedI, deadI int
		var lastErr sql.NullString
		var lastAttempt, nextRetry sql.NullTime
		if err := rows.Scan(&m.RequestID, &m.Topic, &m.TargetServerID, &m.TargetMapID, &m.ProtoID, &m.Payload,
			&sentI, &ackedI, &m.Attempts, &lastErr, &deadI, &m.CreatedAt, &lastAttempt, &nextRetry); err != nil {
			zLog.Error("SQLOutbox scan failed", zap.Error(err))
			continue
		}
		m.Sent = sentI != 0
		m.Acked = ackedI != 0
		m.DeadLetter = deadI != 0
		m.LastError = lastErr.String
		if lastAttempt.Valid {
			m.LastAttemptAt = lastAttempt.Time
		}
		if nextRetry.Valid {
			m.NextRetryAt = nextRetry.Time
		}
		out = append(out, m)
	}
	return out
}

func (o *SQLOutbox) ListPending(limit int) []OutboxMessage {
	return o.query("sent=0 AND dead_letter=0", "ORDER BY created_at ASC", limit)
}

func (o *SQLOutbox) ListRetryable(now time.Time, limit int) []OutboxMessage {
	return o.query("sent=0 AND dead_letter=0 AND acked=0 AND attempts<? AND (next_retry_at IS NULL OR next_retry_at<=?)",
		"ORDER BY next_retry_at IS NULL DESC, next_retry_at ASC, created_at ASC", limit, o.maxRetries, now)
}

func (o *SQLOutbox) ListDeadLetters(limit int) []OutboxMessage {
	return o.query("dead_letter=1", "ORDER BY last_attempt_at ASC", limit)
}

func (o *SQLOutbox) count(where string, args ...interface{}) int {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	var n int
	if err := o.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+o.table+" WHERE "+where, args...).Scan(&n); err != nil {
		zLog.Error("SQLOutbox count failed", zap.String("where", where), zap.Error(err))
		return 0
	}
	return n
}

func (o *SQLOutbox) CountPending() int {
	return o.count("sent=0 AND dead_letter=0")
}

func (o *SQLOutbox) CountRetryable() int {
	return o.count("sent=0 AND dead_letter=0 AND acked=0 AND attempts<? AND (next_retry_at IS NULL OR next_retry_at<=?)",
		o.maxRetries, time.Now())
}

func (o *SQLOutbox) CountDeadLetters() int {
	return o.count("dead_letter=1")
}

func (o *SQLOutbox) PurgeDeadLetters(olderThan time.Duration) int {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	var res sql.Result
	var err error
	if olderThan <= 0 {
		res, err = o.db.ExecContext(ctx, "DELETE FROM "+o.table+" WHERE dead_letter=1")
	} else {
		cutoff := time.Now().Add(-olderThan)
		res, err = o.db.ExecContext(ctx,
			"DELETE FROM "+o.table+" WHERE dead_letter=1 AND COALESCE(last_attempt_at, created_at) <= ?", cutoff)
	}
	if err != nil {
		zLog.Error("SQLOutbox purge failed", zap.Error(err))
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// ---------------- SQLInbox ----------------

type SQLInbox struct {
	db    *sql.DB
	table string
}

func NewSQLInbox(db *sql.DB, table string) *SQLInbox {
	if table == "" {
		table = defaultInboxTable
	}
	return &SQLInbox{db: db, table: table}
}

func (i *SQLInbox) EnsureInboxSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	ddl := "CREATE TABLE IF NOT EXISTS " + i.table + ` (
		request_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
		processed_at DATETIME(3) NOT NULL,
		acked TINYINT NOT NULL DEFAULT 0,
		KEY idx_inbox_processed (processed_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	_, err := i.db.ExecContext(ctx, ddl)
	return err
}

// TryAccept 幂等接受：经主键 INSERT IGNORE 原子去重。首次插入成功→true（应处理），
// 已存在→false（重复，跳过）。DB 出错时保守返回 true（倾向至少一次投递，避免丢消息）。
func (i *SQLInbox) TryAccept(requestID uint64) bool {
	if requestID == 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	res, err := i.db.ExecContext(ctx,
		"INSERT IGNORE INTO "+i.table+" (request_id, processed_at, acked) VALUES (?,?,0)", requestID, time.Now())
	if err != nil {
		zLog.Error("SQLInbox TryAccept failed, defaulting to accept", zap.Uint64("request_id", requestID), zap.Error(err))
		return true
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func (i *SQLInbox) Ack(requestID uint64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	res, err := i.db.ExecContext(ctx, "UPDATE "+i.table+" SET acked=1 WHERE request_id=?", requestID)
	if err != nil {
		zLog.Error("SQLInbox Ack failed", zap.Uint64("request_id", requestID), zap.Error(err))
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// Release 删除去重记录，使同 requestID 可被后续 TryAccept 重新接受（INF-2 失败回滚用）。
func (i *SQLInbox) Release(requestID uint64) {
	if requestID == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	if _, err := i.db.ExecContext(ctx, "DELETE FROM "+i.table+" WHERE request_id=?", requestID); err != nil {
		zLog.Error("SQLInbox Release failed", zap.Uint64("request_id", requestID), zap.Error(err))
	}
}

func (i *SQLInbox) IsProcessed(requestID uint64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	var one int
	err := i.db.QueryRowContext(ctx, "SELECT 1 FROM "+i.table+" WHERE request_id=? LIMIT 1", requestID).Scan(&one)
	if err != nil {
		if err != sql.ErrNoRows {
			zLog.Error("SQLInbox IsProcessed failed", zap.Uint64("request_id", requestID), zap.Error(err))
		}
		return false
	}
	return true
}

func (i *SQLInbox) Cleanup(olderThan time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlOpTimeout)
	defer cancel()
	cutoff := time.Now().Add(-olderThan)
	if _, err := i.db.ExecContext(ctx, "DELETE FROM "+i.table+" WHERE processed_at < ?", cutoff); err != nil {
		zLog.Error("SQLInbox Cleanup failed", zap.Error(err))
	}
}

// ---------------- helpers ----------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}
