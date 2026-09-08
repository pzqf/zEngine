package zConsistency

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const sqlOutboxColumns = "request_id, topic, target_server_id, target_map_id, proto_id, payload, sent, acked, attempts, last_error, dead_letter, created_at, last_attempt_at, next_retry_at"

type rowScanner interface {
	Scan(...any) error
}

func (o *SQLOutbox) EnsureOutboxSchemaContext(ctx context.Context) error {
	if err := checkConsistencyContext(ctx); err != nil {
		return err
	}
	if o == nil || o.db == nil {
		return ErrConsistencyStoreUnavailable
	}
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

func (o *SQLOutbox) Enqueue(ctx context.Context, message OutboxMessage) error {
	if err := validateConsistencyRequest(ctx, message.RequestID); err != nil {
		return err
	}
	if o == nil || o.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	now := time.Now()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	if message.NextRetryAt.IsZero() {
		// 新行不立即"可重投"：重投扫描与首次投递并发，若 next_retry_at=CreatedAt，
		// 扫描会在首个 ACK 在途窗口内把同一消息再发一遍。默认按退避曲线给首投留出
		// ACK 往返窗口；需要更快重试的调用方自行显式设置 NextRetryAt。
		message.NextRetryAt = message.CreatedAt.Add(retryDelay(o.retryBackoff, o.maxRetryDelay, 0))
	}
	setOutboxState(&message, OutboxStateEnqueued)
	payload := append([]byte(nil), message.Payload...)
	_, err := o.db.ExecContext(ctx, "INSERT INTO "+o.table+` (request_id, topic, target_server_id, target_map_id, proto_id, payload,
		sent, acked, attempts, last_error, dead_letter, created_at, last_attempt_at, next_retry_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		message.RequestID, message.Topic, message.TargetServerID, message.TargetMapID, message.ProtoID, payload,
		0, 0, message.Attempts, message.LastError, 0, message.CreatedAt,
		nullTime(message.LastAttemptAt), nullTime(message.NextRetryAt))
	return err
}

func (o *SQLOutbox) Get(ctx context.Context, requestID uint64) (OutboxMessage, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return OutboxMessage{}, err
	}
	if o == nil || o.db == nil {
		return OutboxMessage{}, ErrConsistencyStoreUnavailable
	}
	return scanOutboxRow(o.db.QueryRowContext(ctx,
		"SELECT "+sqlOutboxColumns+" FROM "+o.table+" WHERE request_id=?", requestID))
}

func scanOutboxRow(scanner rowScanner) (OutboxMessage, error) {
	var message OutboxMessage
	var sent, acked, deadLetter int
	var lastError sql.NullString
	var lastAttempt, nextRetry sql.NullTime
	if err := scanner.Scan(&message.RequestID, &message.Topic, &message.TargetServerID, &message.TargetMapID,
		&message.ProtoID, &message.Payload, &sent, &acked, &message.Attempts, &lastError, &deadLetter,
		&message.CreatedAt, &lastAttempt, &nextRetry); err != nil {
		if err == sql.ErrNoRows {
			return OutboxMessage{}, ErrConsistencyEntryNotFound
		}
		return OutboxMessage{}, err
	}
	message.Sent = sent != 0
	message.Acked = acked != 0
	message.DeadLetter = deadLetter != 0
	message.LastError = lastError.String
	if lastAttempt.Valid {
		message.LastAttemptAt = lastAttempt.Time
	}
	if nextRetry.Valid {
		message.NextRetryAt = nextRetry.Time
	}
	return cloneOutboxMessage(message), nil
}

func (o *SQLOutbox) MarkTransported(ctx context.Context, requestID uint64) error {
	return o.transitionContext(ctx, requestID, OutboxStateTransported,
		"UPDATE "+o.table+" SET sent=1 WHERE request_id=? AND sent=0 AND acked=0 AND dead_letter=0")
}

func (o *SQLOutbox) MarkApplied(ctx context.Context, requestID uint64) error {
	return o.transitionContext(ctx, requestID, OutboxStateApplied,
		"UPDATE "+o.table+" SET acked=1 WHERE request_id=? AND sent=1 AND acked=0 AND dead_letter=0")
}

func (o *SQLOutbox) transitionContext(ctx context.Context, requestID uint64, target OutboxState, query string) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	if o == nil || o.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	result, err := o.db.ExecContext(ctx, query, requestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	message, err := o.Get(ctx, requestID)
	if err != nil {
		return err
	}
	current := outboxState(message)
	if current == target {
		return nil
	}
	return transitionError(requestID, current, target)
}

func (o *SQLOutbox) RecordAttempt(ctx context.Context, requestID uint64, cause error) error {
	message, err := o.Get(ctx, requestID)
	if err != nil {
		return err
	}
	state := outboxState(message)
	if state == OutboxStateApplied || state == OutboxStateDeadLetter {
		return transitionError(requestID, state, OutboxStateEnqueued)
	}
	now := time.Now()
	if cause == nil {
		// 成功发送同样要调度下次重投资格：否则 next_retry_at 停留在入队时刻（已过期），
		// 重投扫描会把"ACK 在途"的消息立刻重发一遍。ACK 丢失时按退避曲线（500ms 起）
		// 重投，at-least-once 语义不变，只消除在途窗口内的系统性重复投递。
		nextRetry := now.Add(retryDelay(o.retryBackoff, o.maxRetryDelay, message.Attempts))
		return o.recordAttemptAffected(ctx, requestID,
			"UPDATE "+o.table+" SET attempts=attempts+1, last_attempt_at=?, next_retry_at=? WHERE request_id=? AND acked=0 AND dead_letter=0",
			now, nextRetry, requestID)
	}
	nextRetry := now.Add(retryDelay(o.retryBackoff, o.maxRetryDelay, message.Attempts))
	return o.recordAttemptAffected(ctx, requestID,
		"UPDATE "+o.table+" SET attempts=attempts+1, last_attempt_at=?, last_error=?, sent=0, next_retry_at=? WHERE request_id=? AND acked=0 AND dead_letter=0",
		now, cause.Error(), nextRetry, requestID)
}

func (o *SQLOutbox) MoveToDeadLetter(ctx context.Context, requestID uint64, reason string) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	if o == nil || o.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	now := time.Now()
	result, err := o.db.ExecContext(ctx,
		"UPDATE "+o.table+" SET dead_letter=1, acked=0, last_error=?, last_attempt_at=? WHERE request_id=? AND acked=0",
		reason, now, requestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	message, err := o.Get(ctx, requestID)
	if err != nil {
		return err
	}
	state := outboxState(message)
	if state == OutboxStateDeadLetter {
		return nil
	}
	return transitionError(requestID, state, OutboxStateDeadLetter)
}

func (o *SQLOutbox) Complete(ctx context.Context, requestID uint64, terminal OutboxState) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	if o == nil || o.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	var predicate string
	switch terminal {
	case OutboxStateTransported:
		predicate = "sent=1 AND acked=0 AND dead_letter=0"
	case OutboxStateApplied:
		predicate = "acked=1 AND dead_letter=0"
	default:
		return transitionError(requestID, OutboxStateUnknown, terminal)
	}
	result, err := o.db.ExecContext(ctx,
		"DELETE FROM "+o.table+" WHERE request_id=? AND "+predicate, requestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	message, err := o.Get(ctx, requestID)
	if err != nil {
		return err
	}
	return transitionError(requestID, outboxState(message), terminal)
}

func (o *SQLOutbox) recordAttemptAffected(ctx context.Context, requestID uint64, query string, args ...any) error {
	if err := checkConsistencyContext(ctx); err != nil {
		return err
	}
	if o == nil || o.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	result, err := o.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		message, getErr := o.Get(ctx, requestID)
		if getErr != nil {
			return getErr
		}
		return transitionError(requestID, outboxState(message), OutboxStateEnqueued)
	}
	return nil
}

func (o *SQLOutbox) ListUnresolved(ctx context.Context, limit int) ([]OutboxMessage, error) {
	return o.queryContext(ctx, "dead_letter=0 AND acked=0", "ORDER BY created_at ASC", limit)
}

func (o *SQLOutbox) ListRetryableContext(ctx context.Context, now time.Time, limit int) ([]OutboxMessage, error) {
	return o.queryContext(ctx,
		"dead_letter=0 AND acked=0 AND attempts<? AND (next_retry_at IS NULL OR next_retry_at<=?)",
		"ORDER BY next_retry_at IS NULL DESC, next_retry_at ASC, created_at ASC", limit, o.maxRetries, now)
}

func (o *SQLOutbox) ListDeadLettersContext(ctx context.Context, limit int) ([]OutboxMessage, error) {
	return o.queryContext(ctx, "dead_letter=1", "ORDER BY last_attempt_at ASC", limit)
}

func (o *SQLOutbox) queryContext(ctx context.Context, where, orderBy string, limit int, args ...any) ([]OutboxMessage, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return nil, err
	}
	if o == nil || o.db == nil {
		return nil, ErrConsistencyStoreUnavailable
	}
	args = append(args, normalizedLimit(limit))
	rows, err := o.db.QueryContext(ctx,
		"SELECT "+sqlOutboxColumns+" FROM "+o.table+" WHERE "+where+" "+orderBy+" LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]OutboxMessage, 0, normalizedLimit(limit))
	for rows.Next() {
		message, scanErr := scanOutboxRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (o *SQLOutbox) CountUnresolved(ctx context.Context) (int, error) {
	return o.countContext(ctx, "dead_letter=0 AND acked=0")
}

func (o *SQLOutbox) CountRetryableContext(ctx context.Context, now time.Time) (int, error) {
	return o.countContext(ctx,
		"dead_letter=0 AND acked=0 AND attempts<? AND (next_retry_at IS NULL OR next_retry_at<=?)",
		o.maxRetries, now)
}

func (o *SQLOutbox) CountDeadLettersContext(ctx context.Context) (int, error) {
	return o.countContext(ctx, "dead_letter=1")
}

func (o *SQLOutbox) countContext(ctx context.Context, where string, args ...any) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	if o == nil || o.db == nil {
		return 0, ErrConsistencyStoreUnavailable
	}
	var count int
	if err := o.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+o.table+" WHERE "+where, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (o *SQLOutbox) PurgeDeadLettersContext(ctx context.Context, olderThan time.Duration) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	if o == nil || o.db == nil {
		return 0, ErrConsistencyStoreUnavailable
	}
	query := "DELETE FROM " + o.table + " WHERE dead_letter=1"
	var args []any
	if olderThan > 0 {
		query += " AND COALESCE(last_attempt_at, created_at) <= ?"
		args = append(args, time.Now().Add(-olderThan))
	}
	result, err := o.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}

func (i *SQLInbox) EnsureInboxSchemaContext(ctx context.Context) error {
	if err := checkConsistencyContext(ctx); err != nil {
		return err
	}
	if i == nil || i.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	ddl := "CREATE TABLE IF NOT EXISTS " + i.table + ` (
		request_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
		processed_at DATETIME(3) NOT NULL,
		acked TINYINT NOT NULL DEFAULT 0,
		KEY idx_inbox_processed (processed_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	_, err := i.db.ExecContext(ctx, ddl)
	return err
}

func (i *SQLInbox) Acquire(ctx context.Context, requestID uint64) (InboxAcceptResult, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return InboxAcceptUnknown, err
	}
	if i == nil || i.db == nil {
		return InboxAcceptUnknown, ErrConsistencyStoreUnavailable
	}
	result, err := i.db.ExecContext(ctx,
		"INSERT IGNORE INTO "+i.table+" (request_id, processed_at, acked) VALUES (?,?,0)", requestID, time.Now())
	if err != nil {
		return InboxAcceptUnknown, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return InboxAcceptUnknown, err
	}
	if affected == 1 {
		return InboxAccepted, nil
	}
	state, err := i.Status(ctx, requestID)
	if err != nil {
		return InboxAcceptUnknown, err
	}
	if state == InboxStateProcessed {
		return InboxProcessed, nil
	}
	return InboxInProgress, nil
}

func (i *SQLInbox) Status(ctx context.Context, requestID uint64) (InboxState, error) {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return InboxStateUnknown, err
	}
	if i == nil || i.db == nil {
		return InboxStateUnknown, ErrConsistencyStoreUnavailable
	}
	var acked int
	err := i.db.QueryRowContext(ctx,
		"SELECT acked FROM "+i.table+" WHERE request_id=?", requestID).Scan(&acked)
	if err == sql.ErrNoRows {
		return InboxStateUnknown, ErrConsistencyEntryNotFound
	}
	if err != nil {
		return InboxStateUnknown, err
	}
	if acked != 0 {
		return InboxStateProcessed, nil
	}
	return InboxStateInProgress, nil
}

func (i *SQLInbox) MarkProcessed(ctx context.Context, requestID uint64) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	if i == nil || i.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	result, err := i.db.ExecContext(ctx,
		"UPDATE "+i.table+" SET acked=1, processed_at=? WHERE request_id=? AND acked=0", time.Now(), requestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	state, err := i.Status(ctx, requestID)
	if err != nil {
		return err
	}
	if state == InboxStateProcessed {
		return nil
	}
	return ErrConsistencyEntryNotFound
}

func (i *SQLInbox) Abandon(ctx context.Context, requestID uint64) error {
	if err := validateConsistencyRequest(ctx, requestID); err != nil {
		return err
	}
	if i == nil || i.db == nil {
		return ErrConsistencyStoreUnavailable
	}
	result, err := i.db.ExecContext(ctx,
		"DELETE FROM "+i.table+" WHERE request_id=? AND acked=0", requestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	state, err := i.Status(ctx, requestID)
	if err != nil {
		return err
	}
	if state == InboxStateProcessed {
		return ErrInvalidConsistencyTransition
	}
	return fmt.Errorf("%w: request %d", ErrConsistencyEntryNotFound, requestID)
}

func (i *SQLInbox) CleanupContext(ctx context.Context, olderThan time.Duration) (int, error) {
	if err := checkConsistencyContext(ctx); err != nil {
		return 0, err
	}
	if i == nil || i.db == nil {
		return 0, ErrConsistencyStoreUnavailable
	}
	query := "DELETE FROM " + i.table + " WHERE acked=1"
	var args []any
	if olderThan > 0 {
		query += " AND processed_at < ?"
		args = append(args, time.Now().Add(-olderThan))
	}
	result, err := i.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}
