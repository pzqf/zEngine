package zNet

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/pzqf/zUtil/zCrypto"
)

func TestAESEncryptionDecryption(t *testing.T) {
	tests := []struct {
		name     string
		dataSize int
	}{
		{"小数据", 16},
		{"中等数据", 256},
		{"大数据", 1024},
		{"超大数据", 10240},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, 16)
			rand.Read(key)

			nonce := make([]byte, 12)
			rand.Read(nonce)

			originalData := make([]byte, tt.dataSize)
			rand.Read(originalData)

			encrypted, err := zCrypto.AESEncrypt(originalData, key, nonce, zCrypto.AESModeGCM)
			if err != nil {
				t.Fatalf("AESEncrypt failed: %v", err)
			}

			// 解密时不传递nonce，让函数从加密数据中提取
			decrypted, err := zCrypto.AESDecrypt(encrypted, key, nil, zCrypto.AESModeGCM)
			if err != nil {
				t.Fatalf("AESDecrypt failed: %v", err)
			}

			if !bytes.Equal(decrypted, originalData) {
				t.Errorf("Decrypted data mismatch, original length: %d, decrypted length: %d", len(originalData), len(decrypted))
			}
		})
	}
}

func TestAESEncryptionPerformance(t *testing.T) {
	tests := []struct {
		name     string
		dataSize int
	}{
		{"1KB", 1024},
		{"4KB", 4096},
		{"16KB", 16384},
		{"64KB", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, 16)
			rand.Read(key)

			nonce := make([]byte, 12)
			rand.Read(nonce)

			data := make([]byte, tt.dataSize)
			rand.Read(data)

			t.Logf("Testing encryption/decryption for %s data", tt.name)

			iterations := 100

			start := time.Now()
			for i := 0; i < iterations; i++ {
				encrypted, err := zCrypto.AESEncrypt(data, key, nonce, zCrypto.AESModeGCM)
				if err != nil {
					t.Fatalf("AESEncrypt failed: %v", err)
				}

				_, err = zCrypto.AESDecrypt(encrypted, key, nil, zCrypto.AESModeGCM)
				if err != nil {
					t.Fatalf("AESDecrypt failed: %v", err)
				}
			}
			elapsed := time.Since(start)

			t.Logf("Average time: %v per operation", elapsed/time.Duration(iterations))
			t.Logf("Throughput: %.2f MB/s", float64(tt.dataSize)/elapsed.Seconds()*1000000000/1024/1024*float64(iterations))
		})
	}
}

func TestAESKeyRotation(t *testing.T) {
	key1 := make([]byte, 16)
	rand.Read(key1)

	key2 := make([]byte, 16)
	rand.Read(key2)

	nonce := make([]byte, 12)
	rand.Read(nonce)

	data := make([]byte, 256)
	rand.Read(data)

	encrypted1, err := zCrypto.AESEncrypt(data, key1, nonce, zCrypto.AESModeGCM)
	if err != nil {
		t.Fatalf("AESEncrypt with key1 failed: %v", err)
	}

	encrypted2, err := zCrypto.AESEncrypt(data, key2, nonce, zCrypto.AESModeGCM)
	if err != nil {
		t.Fatalf("AESEncrypt with key2 failed: %v", err)
	}

	if bytes.Equal(encrypted1, encrypted2) {
		t.Errorf("Different keys should produce different encrypted data")
	}

	decrypted1, err := zCrypto.AESDecrypt(encrypted1, key1, nil, zCrypto.AESModeGCM)
	if err != nil {
		t.Fatalf("AESDecrypt with key1 failed: %v", err)
	}

	decrypted2, err := zCrypto.AESDecrypt(encrypted2, key2, nil, zCrypto.AESModeGCM)
	if err != nil {
		t.Fatalf("AESDecrypt with key2 failed: %v", err)
	}

	if !bytes.Equal(decrypted1, data) || !bytes.Equal(decrypted2, data) {
		t.Errorf("Decrypted data mismatch")
	}
}

func BenchmarkAESEncrypt(b *testing.B) {
	key := make([]byte, 16)
	rand.Read(key)

	nonce := make([]byte, 12)
	rand.Read(nonce)

	data := make([]byte, 1024)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = zCrypto.AESEncrypt(data, key, nonce, zCrypto.AESModeGCM)
	}
}

func BenchmarkAESDecrypt(b *testing.B) {
	key := make([]byte, 16)
	rand.Read(key)

	nonce := make([]byte, 12)
	rand.Read(nonce)

	data := make([]byte, 1024)
	rand.Read(data)

	encrypted, _ := zCrypto.AESEncrypt(data, key, nonce, zCrypto.AESModeGCM)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = zCrypto.AESDecrypt(encrypted, key, nil, zCrypto.AESModeGCM)
	}
}

func BenchmarkAESEncrypt4KB(b *testing.B) {
	key := make([]byte, 16)
	rand.Read(key)

	nonce := make([]byte, 12)
	rand.Read(nonce)

	data := make([]byte, 4096)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = zCrypto.AESEncrypt(data, key, nonce, zCrypto.AESModeGCM)
	}
}

func BenchmarkAESDecrypt4KB(b *testing.B) {
	key := make([]byte, 16)
	rand.Read(key)

	nonce := make([]byte, 12)
	rand.Read(nonce)

	data := make([]byte, 4096)
	rand.Read(data)

	encrypted, _ := zCrypto.AESEncrypt(data, key, nonce, zCrypto.AESModeGCM)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = zCrypto.AESDecrypt(encrypted, key, nil, zCrypto.AESModeGCM)
	}
}

func BenchmarkAESEncrypt16KB(b *testing.B) {
	key := make([]byte, 16)
	rand.Read(key)

	nonce := make([]byte, 12)
	rand.Read(nonce)

	data := make([]byte, 16384)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = zCrypto.AESEncrypt(data, key, nonce, zCrypto.AESModeGCM)
	}
}

func BenchmarkAESDecrypt16KB(b *testing.B) {
	key := make([]byte, 16)
	rand.Read(key)

	nonce := make([]byte, 12)
	rand.Read(nonce)

	data := make([]byte, 16384)
	rand.Read(data)

	encrypted, _ := zCrypto.AESEncrypt(data, key, nonce, zCrypto.AESModeGCM)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = zCrypto.AESDecrypt(encrypted, key, nil, zCrypto.AESModeGCM)
	}
}
