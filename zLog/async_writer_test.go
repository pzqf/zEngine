package zLog

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// syncBuf 是并发安全的 zapcore.WriteSyncer 测试替身。
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
func (s *syncBuf) Sync() error { return nil }
func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestAsyncWriter_CloseAndPostCloseWrite 验证 OPT-14：Close 幂等、关闭后 Write 不 panic 且被丢弃
// （不再向无人消费的 buffer 发送）。
func TestAsyncWriter_CloseAndPostCloseWrite(t *testing.T) {
	dst := &syncBuf{}
	aw := NewAsyncWriter(dst, 16, 10*time.Millisecond)

	for i := 0; i < 5; i++ {
		if _, err := aw.Write([]byte("line\n")); err != nil {
			t.Fatalf("write err: %v", err)
		}
	}

	if err := aw.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}
	// 幂等：二次 Close 不 panic、不报错。
	if err := aw.Close(); err != nil {
		t.Fatalf("double close err: %v", err)
	}
	// 关闭后写入不 panic（此前无检查，会向无人消费的 buffer 发送）。
	if _, err := aw.Write([]byte("after-close\n")); err != nil {
		t.Fatalf("post-close write err: %v", err)
	}

	out := dst.String()
	if !bytes.Contains([]byte(out), []byte("line")) {
		t.Fatalf("expected buffered lines flushed on close, got: %q", out)
	}
	if bytes.Contains([]byte(out), []byte("after-close")) {
		t.Fatalf("post-close write must be dropped, but appeared in output: %q", out)
	}
}

// TestAsyncWriter_ConcurrentWriteNoRaceNoPanic 并发写 + Close，-race 下无竞争、无 panic。
func TestAsyncWriter_ConcurrentWriteNoRaceNoPanic(t *testing.T) {
	dst := &syncBuf{}
	aw := NewAsyncWriter(dst, 4, 5*time.Millisecond) // 小缓冲，制造满溢丢弃

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				_, _ = aw.Write([]byte("x\n"))
			}
		}()
	}
	wg.Wait()
	_ = aw.Close()
}
