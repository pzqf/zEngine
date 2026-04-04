package zNet

import (
	"testing"
	"time"
)

func TestKeyRotationManagerGetCurrentKey(t *testing.T) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	key, keyID := krm.GetCurrentKey()
	if len(key) != 16 {
		t.Errorf("Expected key length 16, got %d", len(key))
	}
	if keyID != 1 {
		t.Errorf("Expected initial key ID 1, got %d", keyID)
	}

	t.Logf("Initial key ID: %d", keyID)
}

func TestKeyRotationManagerRotate(t *testing.T) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	_, initialKeyID := krm.GetCurrentKey()
	t.Logf("Initial key ID: %d", initialKeyID)

	newKey, newKeyID := krm.Rotate()

	if newKeyID != initialKeyID+1 {
		t.Errorf("Expected key ID %d, got %d", initialKeyID+1, newKeyID)
	}

	if len(newKey) != 16 {
		t.Errorf("Expected key length 16, got %d", len(newKey))
	}

	_, currentKeyID := krm.GetCurrentKey()
	if currentKeyID != newKeyID {
		t.Errorf("Expected current key ID %d, got %d", newKeyID, currentKeyID)
	}

	t.Logf("Rotated to key ID: %d", newKeyID)
}

func TestKeyRotationManagerMultipleRotations(t *testing.T) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	rotations := 5
	for i := 0; i < rotations; i++ {
		key, keyID := krm.Rotate()
		t.Logf("Rotation %d: key ID %d", i+1, keyID)

		if keyID != uint32(i+2) {
			t.Errorf("Expected key ID %d, got %d", i+2, keyID)
		}

		if len(key) != 16 {
			t.Errorf("Expected key length 16, got %d", len(key))
		}
	}
}

func TestKeyRotationManagerGetKeyByID(t *testing.T) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	_, currentKeyID := krm.GetCurrentKey()

	key, exists := krm.GetKeyByID(currentKeyID)
	if !exists {
		t.Errorf("Expected current key to exist")
	}
	if len(key) != 16 {
		t.Errorf("Expected key length 16, got %d", len(key))
	}

	krm.Rotate()

	oldKey, exists := krm.GetKeyByID(currentKeyID)
	if !exists {
		t.Errorf("Expected old key to exist")
	}
	if len(oldKey) != 16 {
		t.Errorf("Expected old key length 16, got %d", len(oldKey))
	}

	_, exists = krm.GetKeyByID(999)
	if exists {
		t.Errorf("Expected non-existent key to not exist")
	}
}

func TestKeyRotationManagerHistoryLimit(t *testing.T) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	rotations := 10
	for i := 0; i < rotations; i++ {
		krm.Rotate()
	}

	_, currentKeyID := krm.GetCurrentKey()

	for i := uint32(1); i <= currentKeyID; i++ {
		_, exists := krm.GetKeyByID(i)
		if i > currentKeyID-3 {
			if exists {
				t.Errorf("Expected key ID %d to not exist (exceeds history limit)", i)
			}
		} else {
			if !exists {
				t.Errorf("Expected key ID %d to exist", i)
			}
		}
	}

	t.Logf("Current key ID: %d", currentKeyID)
}

func TestKeyRotationManagerShouldRotate(t *testing.T) {
	krm := NewKeyRotationManager(1*time.Second, 3)

	if krm.ShouldRotate() {
		t.Errorf("Should not rotate immediately")
	}

	time.Sleep(1100 * time.Millisecond)

	if !krm.ShouldRotate() {
		t.Errorf("Should rotate after interval")
	}

	krm.Rotate()

	if krm.ShouldRotate() {
		t.Errorf("Should not rotate immediately after rotation")
	}
}

func TestKeyRotationNotifyMarshalUnmarshal(t *testing.T) {
	notify := KeyRotationNotify{
		KeyID:     123,
		Timestamp: 1234567890,
		Nonce:     9876543210,
	}

	data := notify.Marshal()
	if len(data) != 20 {
		t.Errorf("Expected data length 20, got %d", len(data))
	}

	parsed := KeyRotationNotify{}
	if !parsed.Unmarshal(data) {
		t.Errorf("Unmarshal failed")
	}

	if parsed.KeyID != notify.KeyID {
		t.Errorf("Expected KeyID %d, got %d", notify.KeyID, parsed.KeyID)
	}
	if parsed.Timestamp != notify.Timestamp {
		t.Errorf("Expected Timestamp %d, got %d", notify.Timestamp, parsed.Timestamp)
	}
	if parsed.Nonce != notify.Nonce {
		t.Errorf("Expected Nonce %d, got %d", notify.Nonce, parsed.Nonce)
	}
}

func TestKeyRotationNotifyInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"空数据", []byte{}},
		{"短数据", make([]byte, 10)},
		{"错误长度", make([]byte, 19)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notify := KeyRotationNotify{}
			if notify.Unmarshal(tt.data) {
				t.Errorf("Expected unmarshal to fail for %s", tt.name)
			}
		})
	}
}

func BenchmarkKeyRotationManagerRotate(b *testing.B) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = krm.Rotate()
	}
}

func BenchmarkKeyRotationManagerGetCurrentKey(b *testing.B) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = krm.GetCurrentKey()
	}
}

func BenchmarkKeyRotationManagerGetKeyByID(b *testing.B) {
	krm := NewKeyRotationManager(30*time.Minute, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = krm.GetKeyByID(uint32(i % 10))
	}
}

func BenchmarkKeyRotationNotifyMarshal(b *testing.B) {
	notify := KeyRotationNotify{
		KeyID:     123,
		Timestamp: 1234567890,
		Nonce:     9876543210,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = notify.Marshal()
	}
}

func BenchmarkKeyRotationNotifyUnmarshal(b *testing.B) {
	notify := KeyRotationNotify{
		KeyID:     123,
		Timestamp: 1234567890,
		Nonce:     9876543210,
	}
	data := notify.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parsed := KeyRotationNotify{}
		_ = parsed.Unmarshal(data)
	}
}
