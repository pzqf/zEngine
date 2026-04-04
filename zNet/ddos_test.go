package zNet

import (
	"testing"
	"time"
)

func TestDDoSProtectionAllowTraffic(t *testing.T) {
	ddos := NewDDoSProtection()

	tests := []struct {
		name      string
		ip        string
		traffic   int64
		wantAllow bool
	}{
		{"正常流量", "192.168.1.1", 1024, true},
		{"中等流量", "192.168.1.2", 10240, true},
		{"大流量", "192.168.1.3", 102400, true},
		{"超大流量", "192.168.1.4", 11 * 1024 * 1024, false}, // 11MB超过10MB限制
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := ddos.AllowTraffic(tt.ip, tt.traffic)
			if allowed != tt.wantAllow {
				t.Errorf("AllowTraffic(%s, %d) = %v, want %v", tt.ip, tt.traffic, allowed, tt.wantAllow)
			}
		})
	}
}

func TestDDoSProtectionRateLimit(t *testing.T) {
	ddos := NewDDoSProtection()

	ip := "192.168.1.1"

	allowedCount := 0
	for i := 0; i < 200; i++ {
		if ddos.AllowPacket(ip) {
			allowedCount++
		}
	}

	t.Logf("Allowed %d out of 200 requests", allowedCount)

	if allowedCount == 200 {
		t.Errorf("Expected some requests to be rate limited, but all were allowed")
	}
}

func TestDDoSProtectionMultipleIPs(t *testing.T) {
	ddos := NewDDoSProtection()

	ips := []string{
		"192.168.1.1",
		"192.168.1.2",
		"192.168.1.3",
		"192.168.1.4",
		"192.168.1.5",
	}

	totalAllowed := 0
	for i := 0; i < 50; i++ {
		for _, ip := range ips {
			if ddos.AllowPacket(ip) {
				totalAllowed++
			}
		}
	}

	t.Logf("Total allowed requests: %d out of 250", totalAllowed)

	if totalAllowed == 250 {
		t.Errorf("Expected some requests to be rate limited, but all were allowed")
	}
}

func TestDDoSProtectionRecovery(t *testing.T) {
	ddos := NewDDoSProtection()

	ip := "192.168.1.1"

	blocked := false
	for i := 0; i < 200; i++ {
		if !ddos.AllowPacket(ip) {
			blocked = true
			t.Logf("Request %d blocked", i)
			break
		}
	}

	if !blocked {
		t.Errorf("Expected some requests to be blocked")
	}

	t.Logf("Waiting for rate limit to reset...")
	time.Sleep(2 * time.Second)

	allowed := ddos.AllowPacket(ip)
	if !allowed {
		t.Errorf("Expected request to be allowed after reset, but was blocked")
	}

	t.Logf("Request allowed after reset")
}

func TestDDoSProtectionPerformance(t *testing.T) {
	ddos := NewDDoSProtection()

	t.Logf("Testing DDoS protection performance")

	ips := []string{
		"192.168.1.1",
		"192.168.1.2",
		"192.168.1.3",
		"192.168.1.4",
		"192.168.1.5",
	}

	iterations := 10000
	totalAllowed := 0
	totalBlocked := 0

	start := time.Now()
	for i := 0; i < iterations; i++ {
		ip := ips[i%len(ips)]
		if ddos.AllowTraffic(ip, 1024) {
			totalAllowed++
		} else {
			totalBlocked++
		}
	}
	elapsed := time.Since(start)

	t.Logf("Total iterations: %d", iterations)
	t.Logf("Total allowed: %d", totalAllowed)
	t.Logf("Total blocked: %d", totalBlocked)
	t.Logf("Total time: %v", elapsed)
	t.Logf("Throughput: %.2f ops/sec", float64(iterations)/elapsed.Seconds())
	t.Logf("Allowed ratio: %.2f%%", float64(totalAllowed)/float64(iterations)*100)
}

func BenchmarkDDoSProtectionAllowTraffic(b *testing.B) {
	ddos := NewDDoSProtection()

	ip := "192.168.1.1"
	traffic := int64(1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ddos.AllowTraffic(ip, traffic)
	}
}

func BenchmarkDDoSProtectionAllowTrafficMultipleIPs(b *testing.B) {
	ddos := NewDDoSProtection()

	ips := []string{
		"192.168.1.1",
		"192.168.1.2",
		"192.168.1.3",
		"192.168.1.4",
		"192.168.1.5",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := ips[i%len(ips)]
		_ = ddos.AllowTraffic(ip, 1024)
	}
}
