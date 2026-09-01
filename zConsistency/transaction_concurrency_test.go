package zConsistency

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestInMemoryTransactionCoordinatorConcurrentPrepareRunsCallbacksOnce(t *testing.T) {
	var prepareCalls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error {
			prepareCalls.Add(1)
			enteredOnce.Do(func() { close(entered) })
			<-release
			return nil
		},
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
	)
	tx := coordinator.Begin("test", []string{"p1", "p2"}, time.Minute, nil)

	const callers = 32
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- coordinator.Prepare(context.Background(), tx.ID)
		}()
	}
	close(start)
	waitForSignal(t, entered, "prepare callback")
	close(release)
	wg.Wait()
	close(errs)

	var successes int
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful Prepare calls = %d, want 1", successes)
	}
	if got := prepareCalls.Load(); got != 2 {
		t.Fatalf("prepare callback calls = %d, want one per participant", got)
	}
	snapshot, ok := coordinator.GetTransactionSnapshot(tx.ID)
	if !ok || snapshot.State != TransactionStatePrepared || len(snapshot.Prepared) != 2 {
		t.Fatalf("snapshot after prepare = %+v, found=%v", snapshot, ok)
	}
}

func TestInMemoryTransactionCoordinatorConcurrentCommitRunsCallbacksOnce(t *testing.T) {
	var commitCalls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error {
			commitCalls.Add(1)
			enteredOnce.Do(func() { close(entered) })
			<-release
			return nil
		},
		func(context.Context, uint64, string, interface{}) error { return nil },
	)
	tx := coordinator.Begin("test", []string{"p1", "p2"}, time.Minute, nil)
	if err := coordinator.Prepare(context.Background(), tx.ID); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	const callers = 32
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- coordinator.Commit(context.Background(), tx.ID)
		}()
	}
	close(start)
	waitForSignal(t, entered, "commit callback")
	close(release)
	wg.Wait()
	close(errs)

	var successes int
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful Commit calls = %d, want 1", successes)
	}
	if got := commitCalls.Load(); got != 2 {
		t.Fatalf("commit callback calls = %d, want one per participant", got)
	}
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateCommitted {
		t.Fatalf("state = %s, want committed", state)
	}
}

func TestInMemoryTransactionCoordinatorPrepareAbortRollsBackLatePrepare(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var rollbackCalls atomic.Int32
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error {
			close(entered)
			<-release
			return nil
		},
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error {
			rollbackCalls.Add(1)
			return nil
		},
	)
	tx := coordinator.Begin("test", []string{"p1", "p2"}, time.Minute, nil)
	prepareResult := make(chan error, 1)
	go func() {
		prepareResult <- coordinator.Prepare(context.Background(), tx.ID)
	}()

	waitForSignal(t, entered, "prepare callback")
	coordinator.Abort(tx.ID)
	close(release)
	if err := <-prepareResult; err == nil {
		t.Fatal("Prepare() succeeded after concurrent Abort")
	}
	if got := rollbackCalls.Load(); got != 1 {
		t.Fatalf("rollback callback calls = %d, want 1", got)
	}
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateAborted {
		t.Fatalf("state = %s, want aborted", state)
	}
}

func TestInMemoryTransactionCoordinatorPrepareCancellationRollsBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var rollbackCalls atomic.Int32
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error {
			cancel()
			return nil
		},
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error {
			rollbackCalls.Add(1)
			return nil
		},
	)
	tx := coordinator.Begin("test", []string{"p1"}, time.Minute, nil)
	if err := coordinator.Prepare(ctx, tx.ID); !errors.Is(err, ErrTransactionTimeout) {
		t.Fatalf("Prepare() error = %v, want ErrTransactionTimeout", err)
	}
	if got := rollbackCalls.Load(); got != 1 {
		t.Fatalf("rollback callback calls = %d, want 1", got)
	}
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateAborted {
		t.Fatalf("state = %s, want aborted", state)
	}
}

func TestInMemoryTransactionCoordinatorVoteCancellationAbortsBeforePrepare(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var prepareCalls atomic.Int32
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error {
			prepareCalls.Add(1)
			return nil
		},
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
		WithVoteFunc(func(context.Context, uint64, string, interface{}) (VoteResult, error) {
			cancel()
			return VoteCommit, nil
		}),
	)
	tx := coordinator.Begin("test", []string{"p1"}, time.Minute, nil)
	if err := coordinator.Prepare(ctx, tx.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Prepare() error = %v, want context.Canceled", err)
	}
	if got := prepareCalls.Load(); got != 0 {
		t.Fatalf("prepare callback calls = %d, want 0", got)
	}
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateAborted {
		t.Fatalf("state = %s, want aborted", state)
	}
}

func TestInMemoryTransactionCoordinatorAbortDoesNotOverrideCommitting(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var rollbackCalls atomic.Int32
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error {
			close(entered)
			<-release
			return nil
		},
		func(context.Context, uint64, string, interface{}) error {
			rollbackCalls.Add(1)
			return nil
		},
	)
	tx := coordinator.Begin("test", []string{"p1"}, time.Minute, nil)
	if err := coordinator.Prepare(context.Background(), tx.ID); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	commitResult := make(chan error, 1)
	go func() {
		commitResult <- coordinator.Commit(context.Background(), tx.ID)
	}()

	waitForSignal(t, entered, "commit callback")
	coordinator.Abort(tx.ID)
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateCommitting {
		t.Fatalf("state after Abort during commit = %s, want committing", state)
	}
	close(release)
	if err := <-commitResult; err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if got := rollbackCalls.Load(); got != 0 {
		t.Fatalf("rollback callback calls = %d, want 0", got)
	}
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateCommitted {
		t.Fatalf("final state = %s, want committed", state)
	}
}

func TestInMemoryTransactionCoordinatorConcurrentVotesSnapshotsAndCleanup(t *testing.T) {
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
	)
	tx := coordinator.Begin("test", []string{"p1"}, time.Hour, nil)

	const voters = 100
	errs := make(chan error, voters)
	var wg sync.WaitGroup
	for i := 0; i < voters; i++ {
		participant := fmt.Sprintf("p-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- coordinator.Vote(context.Background(), tx.ID, participant, VoteCommit, "ok")
		}()
	}
	for i := 0; i < voters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = coordinator.GetTransactionSnapshot(tx.ID)
			_, _ = coordinator.GetTransactionState(tx.ID)
			_ = coordinator.ActiveCount()
			coordinator.Cleanup(time.Hour)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Vote() error = %v", err)
		}
	}

	snapshot, ok := coordinator.GetTransactionSnapshot(tx.ID)
	if !ok || len(snapshot.Votes) != voters {
		t.Fatalf("snapshot votes = %d, found=%v, want %d", len(snapshot.Votes), ok, voters)
	}
	snapshot.Participants[0] = "changed"
	snapshot.Prepared["changed"] = true
	snapshot.Votes["changed"] = ParticipantVote{Participant: "changed"}
	second, _ := coordinator.GetTransactionSnapshot(tx.ID)
	if second.Participants[0] != "p1" || second.Prepared["changed"] || second.Votes["changed"].Participant != "" {
		t.Fatalf("snapshot mutation leaked into transaction: %+v", second)
	}
}

func TestTransactionManagerCompatibilityConstructor(t *testing.T) {
	legacy := NewTransactionManager(
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
	)
	var coordinator *InMemoryTransactionCoordinator = legacy
	tx := coordinator.Begin("legacy", nil, time.Second, nil)
	if err := coordinator.Prepare(context.Background(), tx.ID); err != nil {
		t.Fatalf("legacy-compatible Prepare() error = %v", err)
	}
	if err := coordinator.Commit(context.Background(), tx.ID); err != nil {
		t.Fatalf("legacy-compatible Commit() error = %v", err)
	}
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateCommitted {
		t.Fatalf("state = %s, want committed", state)
	}
}

func TestInMemoryTransactionCoordinatorVoteAbortIsObservable(t *testing.T) {
	coordinator := NewInMemoryTransactionCoordinator(
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
		func(context.Context, uint64, string, interface{}) error { return nil },
		WithVoteFunc(func(context.Context, uint64, string, interface{}) (VoteResult, error) {
			return VoteAbort, nil
		}),
	)
	tx := coordinator.Begin("test", []string{"p1"}, time.Second, nil)
	err := coordinator.Prepare(context.Background(), tx.ID)
	if !errors.Is(err, ErrVoteRejected) {
		t.Fatalf("Prepare() error = %v, want ErrVoteRejected", err)
	}
	if state, _ := coordinator.GetTransactionState(tx.ID); state != TransactionStateAborted {
		t.Fatalf("state = %s, want aborted", state)
	}
}

func TestInMemoryTransactionCoordinatorCallbacksRunOutsideStateLock(t *testing.T) {
	prepareStates := make(chan TransactionState, 2)
	commitStates := make(chan TransactionState, 1)
	rollbackStates := make(chan TransactionState, 1)
	var coordinator *InMemoryTransactionCoordinator
	coordinator = NewInMemoryTransactionCoordinator(
		func(_ context.Context, txID uint64, _ string, _ interface{}) error {
			state, _ := coordinator.GetTransactionState(txID)
			prepareStates <- state
			return nil
		},
		func(_ context.Context, txID uint64, _ string, _ interface{}) error {
			state, _ := coordinator.GetTransactionState(txID)
			commitStates <- state
			return nil
		},
		func(_ context.Context, txID uint64, _ string, _ interface{}) error {
			state, _ := coordinator.GetTransactionState(txID)
			rollbackStates <- state
			return nil
		},
	)

	committed := coordinator.Begin("commit", []string{"p1"}, time.Second, nil)
	if err := coordinator.Prepare(context.Background(), committed.ID); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := coordinator.Commit(context.Background(), committed.ID); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if state := <-prepareStates; state != TransactionStateVoting {
		t.Fatalf("prepare callback state = %s, want voting", state)
	}
	if state := <-commitStates; state != TransactionStateCommitting {
		t.Fatalf("commit callback state = %s, want committing", state)
	}

	aborted := coordinator.Begin("abort", []string{"p1"}, time.Second, nil)
	if err := coordinator.Prepare(context.Background(), aborted.ID); err != nil {
		t.Fatalf("Prepare() before Abort error = %v", err)
	}
	coordinator.Abort(aborted.ID)
	if state := <-prepareStates; state != TransactionStateVoting {
		t.Fatalf("second prepare callback state = %s, want voting", state)
	}
	if state := <-rollbackStates; state != TransactionStateAborted {
		t.Fatalf("rollback callback state = %s, want aborted", state)
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
