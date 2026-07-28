package zConsistency

import "testing"

// TestMemoryInbox_ReleaseAllowsReaccept 验证 INF-2 修复：TryAccept 占用后，若处理失败调用
// Release 撤销占用，则同 requestID 可被后续 TryAccept 重新接受（重投得以真正重新处理），
// 而不是命中"重复"永远被拒。
func TestMemoryInbox_ReleaseAllowsReaccept(t *testing.T) {
	inbox := NewMemoryInbox()

	const reqID = uint64(12345)

	if !inbox.TryAccept(reqID) {
		t.Fatal("first TryAccept should succeed")
	}
	// 未 Release 前重投应判为重复
	if inbox.TryAccept(reqID) {
		t.Fatal("second TryAccept before Release should be rejected as duplicate")
	}

	// 处理失败 → 撤销占用
	inbox.Release(reqID)

	// 重投应可重新接受并真正处理
	if !inbox.TryAccept(reqID) {
		t.Fatal("TryAccept after Release should succeed (retry can re-process)")
	}

	// 成功后不再 Release，重投恒判重复
	if inbox.TryAccept(reqID) {
		t.Fatal("TryAccept after successful accept (no Release) should be rejected")
	}
}

// TestMemoryInbox_ReleaseUnknownNoop Release 未知/零 ID 不应 panic 或误改计数。
func TestMemoryInbox_ReleaseUnknownNoop(t *testing.T) {
	inbox := NewMemoryInbox()
	inbox.Release(0)
	inbox.Release(999)
	if !inbox.TryAccept(1) {
		t.Fatal("TryAccept should still work after no-op Release")
	}
}
