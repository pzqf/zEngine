package zNet

import (
	"testing"
	"time"
)

func TestSequenceManagerValidateSequence(t *testing.T) {
	sm := NewSequenceManager(1000, 30)

	sid := SessionIdType(12345)

	tests := []struct {
		name      string
		sequence  uint64
		timestamp int64
		wantValid bool
	}{
		{"正常序列号", 1, time.Now().Unix(), true},
		{"递增序列号", 2, time.Now().Unix(), true},
		{"大跨度序列号", 1000, time.Now().Unix(), true},
		{"重复序列号", 1, time.Now().Unix(), false},
		{"过期序列号", 5, time.Now().Unix() - 60, false},
		{"未来序列号", 6, time.Now().Unix() + 60, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := sm.ValidateSequence(sid, tt.sequence, tt.timestamp)
			if valid != tt.wantValid {
				t.Errorf("ValidateSequence(%d, %d) = %v, want %v", tt.sequence, tt.timestamp, valid, tt.wantValid)
			}
		})
	}
}

func TestSequenceManagerWindow(t *testing.T) {
	sm := NewSequenceManager(10, 30)

	sid := SessionIdType(12345)

	sm.ValidateSequence(sid, 1, time.Now().Unix())
	sm.ValidateSequence(sid, 2, time.Now().Unix())
	sm.ValidateSequence(sid, 3, time.Now().Unix())

	if sm.ValidateSequence(sid, 2, time.Now().Unix()) {
		t.Errorf("Expected sequence 2 to be invalid (duplicate)")
	}

	if sm.ValidateSequence(sid, 1, time.Now().Unix()) {
		t.Errorf("Expected sequence 1 to be invalid (duplicate)")
	}

	for i := uint64(4); i <= 20; i++ {
		sm.ValidateSequence(sid, i, time.Now().Unix())
	}

	if sm.ValidateSequence(sid, 5, time.Now().Unix()) {
		t.Errorf("Expected sequence 5 to be invalid (out of window)")
	}
}

func TestSequenceManagerMultipleSessions(t *testing.T) {
	sm := NewSequenceManager(1000, 30)

	sessions := []SessionIdType{1, 2, 3, 4, 5}

	for i, sid := range sessions {
		sm.ValidateSequence(sid, uint64(i+1), time.Now().Unix())
	}

	for i, sid := range sessions {
		valid := sm.ValidateSequence(sid, uint64(i+2), time.Now().Unix())
		if !valid {
			t.Errorf("Expected session %d sequence %d to be valid", sid, i+2)
		}

		invalid := sm.ValidateSequence(sid, uint64(i+1), time.Now().Unix())
		if invalid {
			t.Errorf("Expected session %d sequence %d to be invalid (duplicate)", sid, i+1)
		}
	}
}

func TestSequenceManagerRemoveSession(t *testing.T) {
	sm := NewSequenceManager(1000, 30)

	sid := SessionIdType(12345)

	sm.ValidateSequence(sid, 1, time.Now().Unix())
	sm.ValidateSequence(sid, 2, time.Now().Unix())

	sm.RemoveSession(sid)

	valid := sm.ValidateSequence(sid, 1, time.Now().Unix())
	if !valid {
		t.Errorf("Expected sequence 1 to be valid after session removal")
	}
}

func TestSequenceManagerTimestampTolerance(t *testing.T) {
	sm := NewSequenceManager(1000, 30)

	sid := SessionIdType(12345)

	now := time.Now().Unix()

	valid := sm.ValidateSequence(sid, 1, now)
	if !valid {
		t.Errorf("Expected current timestamp to be valid")
	}

	invalid := sm.ValidateSequence(sid, 2, now-31)
	if invalid {
		t.Errorf("Expected timestamp 31 seconds ago to be invalid")
	}

	invalid = sm.ValidateSequence(sid, 3, now+31)
	if invalid {
		t.Errorf("Expected timestamp 31 seconds in future to be invalid")
	}
}

func TestSequenceManagerPerformance(t *testing.T) {
	sm := NewSequenceManager(1000, 30)

	sessions := make([]SessionIdType, 100)
	for i := 0; i < len(sessions); i++ {
		sessions[i] = SessionIdType(i + 1)
	}

	t.Logf("Testing sequence manager with %d sessions", len(sessions))

	iterations := 10000
	totalValid := 0
	totalInvalid := 0

	start := time.Now()
	for i := 0; i < iterations; i++ {
		sid := sessions[i%len(sessions)]
		seq := uint64(i + 1)
		timestamp := time.Now().Unix()

		if sm.ValidateSequence(sid, seq, timestamp) {
			totalValid++
		} else {
			totalInvalid++
		}
	}
	elapsed := time.Since(start)

	t.Logf("Total iterations: %d", iterations)
	t.Logf("Total valid: %d", totalValid)
	t.Logf("Total invalid: %d", totalInvalid)
	t.Logf("Total time: %v", elapsed)
	t.Logf("Throughput: %.2f ops/sec", float64(iterations)/elapsed.Seconds())
	t.Logf("Valid ratio: %.2f%%", float64(totalValid)/float64(iterations)*100)
}

func BenchmarkSequenceManagerValidateSequence(b *testing.B) {
	sm := NewSequenceManager(1000, 30)

	sid := SessionIdType(12345)
	timestamp := time.Now().Unix()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sm.ValidateSequence(sid, uint64(i+1), timestamp)
	}
}

func BenchmarkSequenceManagerValidateSequenceMultipleSessions(b *testing.B) {
	sm := NewSequenceManager(1000, 30)

	sessions := make([]SessionIdType, 100)
	for i := 0; i < len(sessions); i++ {
		sessions[i] = SessionIdType(i + 1)
	}

	timestamp := time.Now().Unix()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sid := sessions[i%len(sessions)]
		_ = sm.ValidateSequence(sid, uint64(i+1), timestamp)
	}
}

func BenchmarkSequenceManagerRemoveSession(b *testing.B) {
	sm := NewSequenceManager(1000, 30)

	sid := SessionIdType(12345)
	sm.ValidateSequence(sid, 1, time.Now().Unix())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm.RemoveSession(sid)
		sm.ValidateSequence(sid, uint64(i+1), time.Now().Unix())
	}
}
