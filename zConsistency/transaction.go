package zConsistency

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zMap"
	"go.uber.org/zap"
)

var (
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrTransactionAborted  = errors.New("transaction aborted")
	ErrTransactionTimeout  = errors.New("transaction timeout")
	ErrVoteRejected        = errors.New("participant voted reject")
)

type TransactionState int

const (
	TransactionStatePending TransactionState = iota
	TransactionStateVoting
	TransactionStatePrepared
	TransactionStateCommitted
	TransactionStateAborted
	// TransactionStateInconsistent：Prepared 后提交阶段部分参与者提交成功、部分重试耗尽仍失败
	// （2PC 下不能安全回滚已提交者）→ 置此态并告警，等待人工/补偿介入（INF-4）。
	TransactionStateInconsistent
	// TransactionStateCommitting 只表示当前进程已占有提交阶段，不提供崩溃恢复保证。
	// 追加在末尾以保持旧状态值不变。
	TransactionStateCommitting
)

func (s TransactionState) String() string {
	switch s {
	case TransactionStatePending:
		return "pending"
	case TransactionStateVoting:
		return "voting"
	case TransactionStatePrepared:
		return "prepared"
	case TransactionStateCommitted:
		return "committed"
	case TransactionStateAborted:
		return "aborted"
	case TransactionStateInconsistent:
		return "inconsistent"
	case TransactionStateCommitting:
		return "committing"
	default:
		return "unknown"
	}
}

type VoteResult int

const (
	VoteCommit VoteResult = iota
	VoteAbort
)

type ParticipantVote struct {
	Participant string
	Vote        VoteResult
	Reason      string
	Timestamp   time.Time
}

type Transaction struct {
	mu sync.Mutex
	// State、Votes 和 Prepared 为旧 API 保留。协调器运行期间并发读取必须使用 Snapshot；
	// 直接访问这些可变字段无法由协调器代为同步。
	ID           uint64
	State        TransactionState
	Participants []string
	Votes        map[string]*ParticipantVote
	Prepared     map[string]bool
	CreatedAt    time.Time
	Timeout      time.Duration
	Data         interface{}
	Coordinator  string
}

// TransactionSnapshot 是 Transaction 可变状态的并发安全副本。
type TransactionSnapshot struct {
	ID           uint64
	State        TransactionState
	Participants []string
	Votes        map[string]ParticipantVote
	Prepared     map[string]bool
	CreatedAt    time.Time
	Timeout      time.Duration
	Data         interface{}
	Coordinator  string
}

// Snapshot 返回当前事务状态的副本。Data 的具体值仍由调用者拥有，不做深拷贝。
func (tx *Transaction) Snapshot() TransactionSnapshot {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	return tx.snapshotLocked()
}

func (tx *Transaction) snapshotLocked() TransactionSnapshot {
	participants := append([]string(nil), tx.Participants...)
	votes := make(map[string]ParticipantVote, len(tx.Votes))
	for participant, vote := range tx.Votes {
		if vote != nil {
			votes[participant] = *vote
		}
	}
	prepared := make(map[string]bool, len(tx.Prepared))
	for participant, value := range tx.Prepared {
		prepared[participant] = value
	}
	return TransactionSnapshot{
		ID:           tx.ID,
		State:        tx.State,
		Participants: participants,
		Votes:        votes,
		Prepared:     prepared,
		CreatedAt:    tx.CreatedAt,
		Timeout:      tx.Timeout,
		Data:         tx.Data,
		Coordinator:  tx.Coordinator,
	}
}

type PrepareFunc func(ctx context.Context, txID uint64, participant string, data interface{}) error
type CommitFunc func(ctx context.Context, txID uint64, participant string, data interface{}) error
type RollbackFunc func(ctx context.Context, txID uint64, participant string, data interface{}) error
type VoteFunc func(ctx context.Context, txID uint64, participant string, data interface{}) (VoteResult, error)

// InMemoryTransactionCoordinator 是 experimental 的单进程两阶段协调器。
// 它没有 durable log、崩溃恢复、跨进程 fencing 或 exactly-once 保证。
type InMemoryTransactionCoordinator struct {
	transactions *zMap.TypedMap[uint64, *Transaction]
	prepareFn    PrepareFunc
	commitFn     CommitFunc
	rollbackFn   RollbackFunc
	voteFn       VoteFunc
	nextID       atomic.Uint64
	cleanupTick  time.Duration
}

type InMemoryTransactionCoordinatorOption func(*InMemoryTransactionCoordinator)

// TransactionManager 保留旧名称的源兼容。
// Deprecated: use InMemoryTransactionCoordinator.
type TransactionManager = InMemoryTransactionCoordinator

// TransactionManagerOption 保留旧名称的源兼容。
// Deprecated: use InMemoryTransactionCoordinatorOption.
type TransactionManagerOption = InMemoryTransactionCoordinatorOption

func WithVoteFunc(fn VoteFunc) InMemoryTransactionCoordinatorOption {
	return func(tm *InMemoryTransactionCoordinator) {
		tm.voteFn = fn
	}
}

func WithCleanupInterval(d time.Duration) InMemoryTransactionCoordinatorOption {
	return func(tm *InMemoryTransactionCoordinator) {
		tm.cleanupTick = d
	}
}

// NewInMemoryTransactionCoordinator 创建一个只在当前进程内保存状态的 experimental 协调器。
func NewInMemoryTransactionCoordinator(
	prepareFn PrepareFunc,
	commitFn CommitFunc,
	rollbackFn RollbackFunc,
	opts ...InMemoryTransactionCoordinatorOption,
) *InMemoryTransactionCoordinator {
	tm := &InMemoryTransactionCoordinator{
		transactions: zMap.NewTypedMap[uint64, *Transaction](),
		prepareFn:    prepareFn,
		commitFn:     commitFn,
		rollbackFn:   rollbackFn,
		cleanupTick:  30 * time.Second,
	}

	for _, opt := range opts {
		opt(tm)
	}

	return tm
}

// NewTransactionManager 保留旧构造入口。
// Deprecated: use NewInMemoryTransactionCoordinator.
func NewTransactionManager(
	prepareFn PrepareFunc,
	commitFn CommitFunc,
	rollbackFn RollbackFunc,
	opts ...TransactionManagerOption,
) *TransactionManager {
	return NewInMemoryTransactionCoordinator(prepareFn, commitFn, rollbackFn, opts...)
}

func (tm *InMemoryTransactionCoordinator) Begin(coordinator string, participants []string, timeout time.Duration, data interface{}) *Transaction {
	txID := tm.nextID.Add(1)
	tx := &Transaction{
		ID:           txID,
		State:        TransactionStatePending,
		Participants: append([]string(nil), participants...),
		Votes:        make(map[string]*ParticipantVote),
		Prepared:     make(map[string]bool),
		CreatedAt:    time.Now(),
		Timeout:      timeout,
		Data:         data,
		Coordinator:  coordinator,
	}
	tm.transactions.Store(txID, tx)

	zLog.Info("Transaction started",
		zap.Uint64("tx_id", txID),
		zap.String("coordinator", coordinator),
		zap.Strings("participants", participants),
		zap.Duration("timeout", timeout))

	return tx
}

func (tm *InMemoryTransactionCoordinator) Vote(ctx context.Context, txID uint64, participant string, vote VoteResult, reason string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, exists := tm.transactions.Load(txID)
	if !exists {
		return ErrTransactionNotFound
	}

	tx.mu.Lock()
	state := tx.State
	if state != TransactionStatePending && state != TransactionStateVoting && state != TransactionStatePrepared {
		tx.mu.Unlock()
		return fmt.Errorf("transaction %d cannot accept vote in state %s", txID, state)
	}
	tx.Votes[participant] = &ParticipantVote{
		Participant: participant,
		Vote:        vote,
		Reason:      reason,
		Timestamp:   time.Now(),
	}
	var prepared []string
	data := tx.Data
	if vote == VoteAbort {
		tx.State = TransactionStateAborted
		prepared = preparedParticipantsLocked(tx)
	}
	tx.mu.Unlock()

	zLog.Debug("Transaction vote received",
		zap.Uint64("tx_id", txID),
		zap.String("participant", participant),
		zap.Int("vote", int(vote)),
		zap.String("reason", reason))

	if vote == VoteAbort {
		tm.rollbackParticipants(context.WithoutCancel(ctx), txID, prepared, data)
		zLog.Warn("Transaction aborted due to vote reject",
			zap.Uint64("tx_id", txID),
			zap.String("participant", participant),
			zap.String("reason", reason))
	}

	return nil
}

func (tm *InMemoryTransactionCoordinator) Prepare(ctx context.Context, txID uint64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tx, exists := tm.transactions.Load(txID)
	if !exists {
		return ErrTransactionNotFound
	}

	tx.mu.Lock()
	if tx.State != TransactionStatePending {
		state := tx.State
		tx.mu.Unlock()
		return fmt.Errorf("transaction %d is not in pending state: %s", txID, state)
	}
	tx.State = TransactionStateVoting
	participants := append([]string(nil), tx.Participants...)
	data := tx.Data
	tx.mu.Unlock()

	if tm.voteFn != nil {
		for _, participant := range participants {
			select {
			case <-ctx.Done():
				tm.Abort(txID)
				return ErrTransactionTimeout
			default:
			}

			if err := transactionMustBe(tx, TransactionStateVoting); err != nil {
				return err
			}

			vote, err := tm.voteFn(ctx, txID, participant, data)
			if err != nil {
				zLog.Error("Transaction vote request failed",
					zap.Uint64("tx_id", txID),
					zap.String("participant", participant),
					zap.Error(err))
				tm.Abort(txID)
				return fmt.Errorf("vote request failed for %s: %w", participant, err)
			}

			if err := tm.Vote(ctx, txID, participant, vote, "vote callback"); err != nil {
				tm.Abort(txID)
				return err
			}
			if vote == VoteAbort {
				return fmt.Errorf("%s voted abort: %w", participant, ErrVoteRejected)
			}
		}
	}

	if tm.prepareFn == nil {
		tm.Abort(txID)
		return errors.New("transaction prepare function is nil")
	}

	for _, participant := range participants {
		select {
		case <-ctx.Done():
			tm.Abort(txID)
			return ErrTransactionTimeout
		default:
		}

		if err := transactionMustBe(tx, TransactionStateVoting); err != nil {
			return err
		}

		if err := tm.prepareFn(ctx, txID, participant, data); err != nil {
			zLog.Error("Transaction prepare failed",
				zap.Uint64("tx_id", txID),
				zap.String("participant", participant),
				zap.Error(err))
			tm.Abort(txID)
			return fmt.Errorf("prepare failed for %s: %w", participant, err)
		}

		tx.mu.Lock()
		if tx.State != TransactionStateVoting {
			state := tx.State
			tx.mu.Unlock()
			tm.rollbackParticipants(context.WithoutCancel(ctx), txID, []string{participant}, data)
			return fmt.Errorf("transaction %d left voting state while preparing %s: %s", txID, participant, state)
		}
		tx.Prepared[participant] = true
		tx.mu.Unlock()

		zLog.Debug("Transaction participant prepared",
			zap.Uint64("tx_id", txID),
			zap.String("participant", participant))
		if ctx.Err() != nil {
			tm.Abort(txID)
			return ErrTransactionTimeout
		}
	}

	tx.mu.Lock()
	if ctx.Err() != nil {
		tx.mu.Unlock()
		tm.Abort(txID)
		return ErrTransactionTimeout
	}
	if tx.State != TransactionStateVoting {
		state := tx.State
		tx.mu.Unlock()
		return fmt.Errorf("transaction %d cannot finish prepare from state %s", txID, state)
	}
	tx.State = TransactionStatePrepared
	tx.mu.Unlock()

	zLog.Info("Transaction prepared", zap.Uint64("tx_id", txID))
	return nil
}

func (tm *InMemoryTransactionCoordinator) Commit(ctx context.Context, txID uint64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tx, exists := tm.transactions.Load(txID)
	if !exists {
		return ErrTransactionNotFound
	}

	tx.mu.Lock()
	if tx.State != TransactionStatePrepared {
		state := tx.State
		tx.mu.Unlock()
		return fmt.Errorf("transaction %d is not in prepared state: %s", txID, state)
	}
	tx.State = TransactionStateCommitting
	participants := append([]string(nil), tx.Participants...)
	data := tx.Data
	tx.mu.Unlock()

	// INF-4: 2PC 一旦进入 Prepared，所有参与者已投票同意，提交阶段必须对每个参与者最终成功——
	// 不能中途因 ctx 取消而 break 放弃（原代码 `break` 只跳出 select 而非 for，实际根本没停；
	// 且部分失败被误标 Aborted 暗示已回滚，实则已提交者未回滚）。改为：对失败的提交做有界重试；
	// 仍失败则标记为需人工/补偿介入的不一致态，不误标 Aborted。
	const maxCommitAttempts = 3
	var commitErrors []error
	for _, participant := range participants {
		var lastErr error
		for attempt := 0; attempt < maxCommitAttempts; attempt++ {
			if tm.commitFn == nil {
				lastErr = errors.New("transaction commit function is nil")
			} else {
				lastErr = tm.commitFn(ctx, txID, participant, data)
			}
			if lastErr == nil {
				break
			}
			zLog.Warn("Transaction commit attempt failed, retrying",
				zap.Uint64("tx_id", txID),
				zap.String("participant", participant),
				zap.Int("attempt", attempt+1),
				zap.Error(lastErr))
		}
		if lastErr != nil {
			commitErrors = append(commitErrors, fmt.Errorf("%s: %w", participant, lastErr))
			zLog.Error("Transaction commit failed for participant after retries",
				zap.Uint64("tx_id", txID),
				zap.String("participant", participant),
				zap.Error(lastErr))
		}
	}

	if len(commitErrors) > 0 {
		// 部分参与者已提交、部分重试耗尽仍失败：2PC 下不能安全回滚已提交者。
		tx.mu.Lock()
		tx.State = TransactionStateInconsistent
		tx.mu.Unlock()
		zLog.Error("Transaction left INCONSISTENT: some participants committed, others failed after retries — needs manual/compensation intervention",
			zap.Uint64("tx_id", txID), zap.Int("failed_participants", len(commitErrors)))
		return fmt.Errorf("commit errors (transaction inconsistent, needs intervention): %v", commitErrors)
	}

	tx.mu.Lock()
	tx.State = TransactionStateCommitted
	tx.mu.Unlock()
	zLog.Info("Transaction committed", zap.Uint64("tx_id", txID))
	return nil
}

func (tm *InMemoryTransactionCoordinator) Abort(txID uint64) {
	tx, exists := tm.transactions.Load(txID)
	if !exists {
		return
	}

	prepared, data, transitioned := transitionToAborted(tx)
	if !transitioned {
		return
	}
	tm.rollbackParticipants(context.Background(), txID, prepared, data)

	zLog.Info("Transaction aborted", zap.Uint64("tx_id", txID))
}

func (tm *InMemoryTransactionCoordinator) Execute(ctx context.Context, coordinator string, participants []string, timeout time.Duration, data interface{}) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tx := tm.Begin(coordinator, participants, timeout, data)

	txCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := tm.Prepare(txCtx, tx.ID); err != nil {
		return fmt.Errorf("prepare phase failed: %w", err)
	}

	if err := tm.Commit(txCtx, tx.ID); err != nil {
		return fmt.Errorf("commit phase failed: %w", err)
	}

	return nil
}

// GetTransaction 返回旧的可变事务对象。并发读取可变字段时必须调用 Transaction.Snapshot。
func (tm *InMemoryTransactionCoordinator) GetTransaction(txID uint64) (*Transaction, bool) {
	return tm.transactions.Load(txID)
}

func (tm *InMemoryTransactionCoordinator) GetTransactionSnapshot(txID uint64) (TransactionSnapshot, bool) {
	tx, exists := tm.transactions.Load(txID)
	if !exists {
		return TransactionSnapshot{}, false
	}
	return tx.Snapshot(), true
}

func (tm *InMemoryTransactionCoordinator) GetTransactionState(txID uint64) (TransactionState, bool) {
	tx, exists := tm.transactions.Load(txID)
	if !exists {
		return TransactionStatePending, false
	}
	tx.mu.Lock()
	state := tx.State
	tx.mu.Unlock()
	return state, true
}

func (tm *InMemoryTransactionCoordinator) Cleanup(maxAge time.Duration) {
	now := time.Now()
	var toDelete []uint64

	tm.transactions.Range(func(id uint64, tx *Transaction) bool {
		tx.mu.Lock()
		age := now.Sub(tx.CreatedAt)
		state := tx.State
		timedOut := state == TransactionStatePending && age > tx.Timeout
		if timedOut {
			tx.State = TransactionStateAborted
			state = TransactionStateAborted
		}
		if (state == TransactionStateCommitted || state == TransactionStateAborted) &&
			(age > maxAge || timedOut) {
			toDelete = append(toDelete, id)
		}
		tx.mu.Unlock()
		if timedOut {
			zLog.Warn("Cleaning up timed out transaction",
				zap.Uint64("tx_id", id),
				zap.Duration("age", age))
		}
		return true
	})

	for _, id := range toDelete {
		tm.transactions.Delete(id)
	}

	if len(toDelete) > 0 {
		zLog.Debug("Cleaned up transactions", zap.Int("count", len(toDelete)))
	}
}

func (tm *InMemoryTransactionCoordinator) StartCleanupLoop(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(tm.cleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.Cleanup(5 * time.Minute)
		}
	}
}

func (tm *InMemoryTransactionCoordinator) ActiveCount() int {
	count := 0
	tm.transactions.Range(func(id uint64, tx *Transaction) bool {
		tx.mu.Lock()
		state := tx.State
		tx.mu.Unlock()
		if state == TransactionStatePending || state == TransactionStateVoting ||
			state == TransactionStatePrepared || state == TransactionStateCommitting {
			count++
		}
		return true
	})
	return count
}

func (tm *InMemoryTransactionCoordinator) TotalCount() int {
	return int(tm.transactions.Len())
}

func transactionMustBe(tx *Transaction, required TransactionState) error {
	tx.mu.Lock()
	state := tx.State
	tx.mu.Unlock()
	if state == required {
		return nil
	}
	if state == TransactionStateAborted {
		return fmt.Errorf("transaction %d is aborted: %w", tx.ID, ErrTransactionAborted)
	}
	return fmt.Errorf("transaction %d is in state %s, want %s", tx.ID, state, required)
}

func transitionToAborted(tx *Transaction) ([]string, interface{}, bool) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	switch tx.State {
	case TransactionStatePending, TransactionStateVoting, TransactionStatePrepared:
		tx.State = TransactionStateAborted
		return preparedParticipantsLocked(tx), tx.Data, true
	default:
		return nil, nil, false
	}
}

func preparedParticipantsLocked(tx *Transaction) []string {
	prepared := make([]string, 0, len(tx.Prepared))
	for _, participant := range tx.Participants {
		if tx.Prepared[participant] {
			prepared = append(prepared, participant)
		}
	}
	return prepared
}

func (tm *InMemoryTransactionCoordinator) rollbackParticipants(
	ctx context.Context,
	txID uint64,
	participants []string,
	data interface{},
) {
	if len(participants) == 0 {
		return
	}
	if tm.rollbackFn == nil {
		zLog.Error("Transaction rollback function is nil", zap.Uint64("tx_id", txID))
		return
	}
	for _, participant := range participants {
		if err := tm.rollbackFn(ctx, txID, participant, data); err != nil {
			zLog.Error("Transaction rollback failed",
				zap.Uint64("tx_id", txID),
				zap.String("participant", participant),
				zap.Error(err))
		}
	}
}
