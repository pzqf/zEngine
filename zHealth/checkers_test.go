package zHealth

import (
	"runtime"
	"testing"
)

func TestPlaceholderCheckersReturnUnknown(t *testing.T) {
	for name, checker := range map[string]HealthChecker{
		"disk": NewDiskChecker(),
		"time": NewTimeChecker(),
	} {
		status, _, err := checker.Check()
		if err != nil {
			t.Fatalf("%s checker error: %v", name, err)
		}
		if status != HealthStatusUnknown {
			t.Fatalf("%s status = %s, want unknown", name, status)
		}
	}
}

func TestGCCheckerDoesNotForceCollection(t *testing.T) {
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	status, _, err := NewGCChecker().Check()
	if err != nil {
		t.Fatal(err)
	}
	if status == HealthStatusUnknown {
		t.Fatal("GC checker should expose read-only runtime diagnostics")
	}

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.NumForcedGC != before.NumForcedGC {
		t.Fatalf("NumForcedGC changed from %d to %d", before.NumForcedGC, after.NumForcedGC)
	}
}
