package zMetrics

import (
	"errors"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsManagerCheckedRegistrationReusesExactSchema(t *testing.T) {
	mgr := NewMetricsManager()
	labels := map[string]string{"role": "map", "realm": "0001"}

	first, err := mgr.RegisterCounterChecked("requests_total", "accepted requests", labels)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	labels["role"] = "mutated"
	second, err := mgr.RegisterCounterChecked(
		"requests_total",
		"accepted requests",
		map[string]string{"realm": "0001", "role": "map"},
	)
	if err != nil {
		t.Fatalf("same schema registration failed: %v", err)
	}
	if first != second {
		t.Fatal("same schema did not return the registered collector")
	}
	if _, err := mgr.RegisterCounterChecked("requests_total", "accepted requests", map[string]string{}); !errors.Is(err, ErrMetricSchemaConflict) {
		t.Fatalf("removing const labels error = %v, want ErrMetricSchemaConflict", err)
	}

	withoutLabels, err := mgr.RegisterGaugeChecked("workers", "workers", nil)
	if err != nil {
		t.Fatalf("register nil-label gauge: %v", err)
	}
	withEmptyLabels, err := mgr.RegisterGaugeChecked("workers", "workers", map[string]string{})
	if err != nil {
		t.Fatalf("empty labels should equal nil labels: %v", err)
	}
	if withoutLabels != withEmptyLabels {
		t.Fatal("nil and empty labels did not reuse collector")
	}

	config, ok := mgr.GetMetricConfig("requests_total")
	if !ok {
		t.Fatal("registered metric config is missing")
	}
	if got := config.Labels["role"]; got != "map" {
		t.Fatalf("stored schema aliases caller labels: got %q", got)
	}
}

func TestMetricsManagerCheckedRegistrationRejectsSchemaConflictsWithoutGhosts(t *testing.T) {
	tests := []struct {
		name     string
		register func(*MetricsManager) error
	}{
		{
			name: "type",
			register: func(mgr *MetricsManager) error {
				_, err := mgr.RegisterGaugeChecked("schema_metric", "schema help", map[string]string{"role": "game"})
				return err
			},
		},
		{
			name: "help",
			register: func(mgr *MetricsManager) error {
				_, err := mgr.RegisterCounterChecked("schema_metric", "different help", map[string]string{"role": "game"})
				return err
			},
		},
		{
			name: "label value",
			register: func(mgr *MetricsManager) error {
				_, err := mgr.RegisterCounterChecked("schema_metric", "schema help", map[string]string{"role": "map"})
				return err
			},
		},
		{
			name: "label name",
			register: func(mgr *MetricsManager) error {
				_, err := mgr.RegisterCounterChecked("schema_metric", "schema help", map[string]string{"server": "game"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewMetricsManager()
			original, err := mgr.RegisterCounterChecked(
				"schema_metric",
				"schema help",
				map[string]string{"role": "game"},
			)
			if err != nil {
				t.Fatalf("register original: %v", err)
			}

			err = tt.register(mgr)
			if !errors.Is(err, ErrMetricSchemaConflict) {
				t.Fatalf("conflict error = %v, want ErrMetricSchemaConflict", err)
			}
			stored, ok := mgr.GetCounter("schema_metric")
			if !ok || stored != original {
				t.Fatal("schema conflict replaced or removed the registered collector")
			}
			if _, ok := mgr.GetGauge("schema_metric"); ok {
				t.Fatal("cross-type conflict cached an unregistered gauge")
			}
		})
	}
}

func TestMetricsManagerCheckedHistogramComparesEffectiveBuckets(t *testing.T) {
	mgr := NewMetricsManager()
	first, err := mgr.RegisterHistogramChecked("latency_seconds", "latency", nil, nil)
	if err != nil {
		t.Fatalf("register default histogram: %v", err)
	}
	second, err := mgr.RegisterHistogramChecked("latency_seconds", "latency", prometheus.DefBuckets, nil)
	if err != nil {
		t.Fatalf("register equivalent default buckets: %v", err)
	}
	if first != second {
		t.Fatal("equivalent default buckets did not reuse collector")
	}
	if _, err := mgr.RegisterHistogramChecked("latency_seconds", "latency", []float64{0.1, 1}, nil); !errors.Is(err, ErrMetricSchemaConflict) {
		t.Fatalf("bucket conflict error = %v, want ErrMetricSchemaConflict", err)
	}
	if _, err := mgr.RegisterHistogramChecked("invalid_buckets", "invalid", []float64{1, 1}, nil); !errors.Is(err, ErrInvalidMetricSchema) {
		t.Fatalf("invalid bucket error = %v, want ErrInvalidMetricSchema", err)
	}
	if _, ok := mgr.GetHistogram("invalid_buckets"); ok {
		t.Fatal("invalid histogram was cached")
	}
	if _, err := mgr.RegisterHistogramChecked("reserved_label", "invalid", nil, map[string]string{"le": "1"}); !errors.Is(err, ErrInvalidMetricSchema) {
		t.Fatalf("reserved histogram label error = %v, want ErrInvalidMetricSchema", err)
	}
	if _, ok := mgr.GetHistogram("reserved_label"); ok {
		t.Fatal("histogram with reserved label was cached")
	}
}

func TestMetricsManagerCheckedRegistrationDoesNotAdoptExternalCollector(t *testing.T) {
	mgr := NewMetricsManager()
	external := prometheus.NewGauge(prometheus.GaugeOpts{Name: "external_metric", Help: "external"})
	if err := mgr.GetRegistry().Register(external); err != nil {
		t.Fatalf("register external collector: %v", err)
	}

	if _, err := mgr.RegisterGaugeChecked("external_metric", "external", nil); !errors.Is(err, ErrMetricRegistration) {
		t.Fatalf("external conflict error = %v, want ErrMetricRegistration", err)
	}
	if _, ok := mgr.GetGauge("external_metric"); ok {
		t.Fatal("externally registered collector was cached without manager schema ownership")
	}
}

func TestMetricsManagerCheckedRegistrationIsConcurrentAndSingular(t *testing.T) {
	mgr := NewMetricsManager()
	const callers = 64
	collectors := make(chan prometheus.Counter, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collector, err := mgr.RegisterCounterChecked("concurrent_total", "concurrent", map[string]string{"scope": "test"})
			collectors <- collector
			errs <- err
		}()
	}
	wg.Wait()
	close(collectors)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent registration failed: %v", err)
		}
	}
	var first prometheus.Counter
	for collector := range collectors {
		if first == nil {
			first = collector
			continue
		}
		if collector != first {
			t.Fatal("concurrent registration returned multiple collectors")
		}
	}
	families, err := mgr.GetRegistry().Gather()
	if err != nil {
		t.Fatalf("gather registry: %v", err)
	}
	if len(families) != 1 || families[0].GetName() != "concurrent_total" {
		t.Fatalf("registered metric families = %v, want only concurrent_total", families)
	}
}

func TestMetricsManagerResetNetworkMetricsNamesItsScope(t *testing.T) {
	mgr := NewMetricsManager()
	mgr.GetNetworkMetrics().IncDroppedPackets()
	mgr.ResetNetworkMetrics()
	if got := mgr.GetNetworkMetrics().GetDroppedPackets(); got != 0 {
		t.Fatalf("dropped packets after reset = %d, want 0", got)
	}
}
