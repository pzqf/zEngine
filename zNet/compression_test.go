package zNet

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/golang/snappy"
)

func TestSnappyCompression(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		expectCompress bool
	}{
		{"小数据-不可压缩", func() []byte { data := make([]byte, 16); rand.Read(data); return data }(), false},
		{"小数据-可压缩", bytes.Repeat([]byte("test"), 32), true},
		{"中等数据", bytes.Repeat([]byte("test pattern"), 64), true},
		{"大数据", bytes.Repeat([]byte("repeated data pattern"), 1024), true},
		{"超大数据", bytes.Repeat([]byte("long repeated data pattern for compression test"), 16384), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed := snappy.Encode(nil, tt.data)
			decompressed, err := snappy.Decode(nil, compressed)

			if err != nil {
				t.Fatalf("Snappy decode failed: %v", err)
			}

			if !bytes.Equal(decompressed, tt.data) {
				t.Errorf("Decompressed data mismatch, original size: %d, decompressed size: %d", len(tt.data), len(decompressed))
			}

			compressionRatio := float64(len(compressed)) / float64(len(tt.data)) * 100
			t.Logf("Original size: %d, Compressed size: %d, Ratio: %.2f%%", len(tt.data), len(compressed), compressionRatio)

			if tt.expectCompress && len(compressed) >= len(tt.data) {
				t.Errorf("Expected compression but got larger size: %d >= %d", len(compressed), len(tt.data))
			}
		})
	}
}

func TestSnappyCompressionPerformance(t *testing.T) {
	tests := []struct {
		name     string
		dataSize int
	}{
		{"1KB", 1024},
		{"4KB", 4096},
		{"16KB", 16384},
		{"64KB", 65536},
		{"256KB", 262144},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.dataSize)
			rand.Read(data)

			t.Logf("Testing compression for %s data", tt.name)

			iterations := 100

			start := time.Now()
			for i := 0; i < iterations; i++ {
				compressed := snappy.Encode(nil, data)
				_, err := snappy.Decode(nil, compressed)
				if err != nil {
					t.Fatalf("Snappy decode failed: %v", err)
				}
			}
			elapsed := time.Since(start)

			compressed := snappy.Encode(nil, data)
			compressionRatio := float64(len(compressed)) / float64(len(data)) * 100

			t.Logf("Average time: %v per operation", elapsed/time.Duration(iterations))
			t.Logf("Compression ratio: %.2f%%", compressionRatio)
			t.Logf("Throughput: %.2f MB/s", float64(tt.dataSize)/elapsed.Seconds()*1000000000/1024/1024*float64(iterations))
		})
	}
}

func TestSnappyWithRealData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"重复数据", bytes.Repeat([]byte("test data pattern"), 100)},
		{"JSON数据", []byte(`{"id":12345,"name":"test","value":123.45,"data":"test data"}`)},
		{"二进制数据", func() []byte {
			data := make([]byte, 256)
			for i := 0; i < len(data); i++ {
				data[i] = byte(i)
			}
			return data
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed := snappy.Encode(nil, tt.data)
			decompressed, err := snappy.Decode(nil, compressed)

			if err != nil {
				t.Fatalf("Snappy decode failed: %v", err)
			}

			if !bytes.Equal(decompressed, tt.data) {
				t.Errorf("Decompressed data mismatch")
			}

			compressionRatio := float64(len(compressed)) / float64(len(tt.data)) * 100
			t.Logf("%s - Original: %d, Compressed: %d, Ratio: %.2f%%", tt.name, len(tt.data), len(compressed), compressionRatio)
		})
	}
}

func BenchmarkSnappyEncode1KB(b *testing.B) {
	data := make([]byte, 1024)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = snappy.Encode(nil, data)
	}
}

func BenchmarkSnappyDecode1KB(b *testing.B) {
	data := make([]byte, 1024)
	rand.Read(data)
	compressed := snappy.Encode(nil, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = snappy.Decode(nil, compressed)
	}
}

func BenchmarkSnappyEncode4KB(b *testing.B) {
	data := make([]byte, 4096)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = snappy.Encode(nil, data)
	}
}

func BenchmarkSnappyDecode4KB(b *testing.B) {
	data := make([]byte, 4096)
	rand.Read(data)
	compressed := snappy.Encode(nil, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = snappy.Decode(nil, compressed)
	}
}

func BenchmarkSnappyEncode16KB(b *testing.B) {
	data := make([]byte, 16384)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = snappy.Encode(nil, data)
	}
}

func BenchmarkSnappyDecode16KB(b *testing.B) {
	data := make([]byte, 16384)
	rand.Read(data)
	compressed := snappy.Encode(nil, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = snappy.Decode(nil, compressed)
	}
}

func BenchmarkSnappyEncode64KB(b *testing.B) {
	data := make([]byte, 65536)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = snappy.Encode(nil, data)
	}
}

func BenchmarkSnappyDecode64KB(b *testing.B) {
	data := make([]byte, 65536)
	rand.Read(data)
	compressed := snappy.Encode(nil, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = snappy.Decode(nil, compressed)
	}
}
