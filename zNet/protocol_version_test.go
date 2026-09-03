package zNet

import (
	"errors"
	"sync"
	"testing"
)

func TestProtocolVersionPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  ProtocolVersionPolicy
		wantErr bool
	}{
		{name: "disabled zero value", policy: ProtocolVersionPolicy{}},
		{name: "enabled single version", policy: ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 1}},
		{name: "enabled range", policy: ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 3, Capabilities: 7, AcceptLegacy: true}},
		{name: "disabled range", policy: ProtocolVersionPolicy{MinVersion: 1}, wantErr: true},
		{name: "disabled capabilities", policy: ProtocolVersionPolicy{Capabilities: 1}, wantErr: true},
		{name: "disabled legacy flag", policy: ProtocolVersionPolicy{AcceptLegacy: true}, wantErr: true},
		{name: "zero minimum", policy: ProtocolVersionPolicy{Enabled: true, MinVersion: 0, MaxVersion: 1}, wantErr: true},
		{name: "negative minimum", policy: ProtocolVersionPolicy{Enabled: true, MinVersion: -1, MaxVersion: 1}, wantErr: true},
		{name: "reversed range", policy: ProtocolVersionPolicy{Enabled: true, MinVersion: 3, MaxVersion: 2}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.Validate()
			if test.wantErr && !errors.Is(err, ErrInvalidProtocolVersionPolicy) {
				t.Fatalf("Validate() error = %v, want ErrInvalidProtocolVersionPolicy", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestNegotiateProtocolCompatibility(t *testing.T) {
	policy := func(minimum, maximum int32, capabilities uint64, acceptLegacy bool) ProtocolVersionPolicy {
		return ProtocolVersionPolicy{
			Enabled:      true,
			MinVersion:   minimum,
			MaxVersion:   maximum,
			Capabilities: capabilities,
			AcceptLegacy: acceptLegacy,
		}
	}

	tests := []struct {
		name    string
		local   ProtocolVersionPolicy
		peer    ProtocolVersionPolicy
		want    ProtocolCompatibility
		wantErr error
	}{
		{
			name: "both legacy",
			want: ProtocolCompatibility{Version: LegacyProtocolVersion},
		},
		{
			name:  "enabled accepts legacy peer",
			local: policy(1, 2, 0b111, true),
			want:  ProtocolCompatibility{Version: LegacyProtocolVersion},
		},
		{
			name: "legacy local accepted by enabled peer",
			peer: policy(1, 2, 0b111, true),
			want: ProtocolCompatibility{Version: LegacyProtocolVersion},
		},
		{
			name:    "legacy rejected",
			local:   policy(1, 2, 0, false),
			wantErr: ErrNoCompatibleProtocolVersion,
		},
		{
			name:  "highest overlap and capability intersection",
			local: policy(2, 5, 0b1110, false),
			peer:  policy(3, 4, 0b1011, false),
			want:  ProtocolCompatibility{Version: 4, Capabilities: 0b1010},
		},
		{
			name:  "touching ranges",
			local: policy(1, 3, 0b11, false),
			peer:  policy(3, 6, 0b10, false),
			want:  ProtocolCompatibility{Version: 3, Capabilities: 0b10},
		},
		{
			name:    "disjoint ranges",
			local:   policy(1, 2, 0, false),
			peer:    policy(3, 4, 0, false),
			wantErr: ErrNoCompatibleProtocolVersion,
		},
		{
			name:    "invalid local",
			local:   ProtocolVersionPolicy{Enabled: true, MinVersion: 2, MaxVersion: 1},
			peer:    policy(1, 2, 0, false),
			wantErr: ErrInvalidProtocolVersionPolicy,
		},
		{
			name:    "invalid peer",
			local:   policy(1, 2, 0, false),
			peer:    ProtocolVersionPolicy{Enabled: true, MinVersion: 0, MaxVersion: 2},
			wantErr: ErrInvalidProtocolVersionPolicy,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NegotiateProtocolCompatibility(test.local, test.peer)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("NegotiateProtocolCompatibility() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NegotiateProtocolCompatibility() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("NegotiateProtocolCompatibility() = %+v, want %+v", got, test.want)
			}

			reversed, err := NegotiateProtocolCompatibility(test.peer, test.local)
			if err != nil {
				t.Fatalf("reversed negotiation error = %v", err)
			}
			if reversed != got {
				t.Fatalf("reversed negotiation = %+v, want %+v", reversed, got)
			}
		})
	}
}

func TestProtocolCompatibilityRequireCapabilities(t *testing.T) {
	compatibility := ProtocolCompatibility{Version: 2, Capabilities: 0b1011}
	if compatibility.IsLegacy() {
		t.Fatal("positive version reported as legacy")
	}
	if !compatibility.SupportsAll(0b0011) {
		t.Fatal("SupportsAll() rejected available capabilities")
	}
	if err := compatibility.RequireCapabilities(0b0011); err != nil {
		t.Fatalf("RequireCapabilities() error = %v", err)
	}
	if err := compatibility.RequireCapabilities(0b0101); !errors.Is(err, ErrProtocolCapabilitiesUnavailable) {
		t.Fatalf("RequireCapabilities() error = %v, want ErrProtocolCapabilitiesUnavailable", err)
	}

	legacy := ProtocolCompatibility{Version: LegacyProtocolVersion}
	if !legacy.IsLegacy() {
		t.Fatal("Version=0 did not report legacy")
	}
	if err := legacy.RequireCapabilities(0); err != nil {
		t.Fatalf("legacy RequireCapabilities(0) error = %v", err)
	}
	if err := legacy.RequireCapabilities(1); !errors.Is(err, ErrProtocolCapabilitiesUnavailable) {
		t.Fatalf("legacy RequireCapabilities(1) error = %v", err)
	}
}

func TestProtocolVersionNegotiatorSnapshotAndFailureIsolation(t *testing.T) {
	local := ProtocolVersionPolicy{Enabled: true, MinVersion: 2, MaxVersion: 4, Capabilities: 0b1111}
	negotiator, err := NewProtocolVersionNegotiator(local)
	if err != nil {
		t.Fatalf("NewProtocolVersionNegotiator() error = %v", err)
	}
	if got := negotiator.LocalPolicy(); got != local {
		t.Fatalf("LocalPolicy() = %+v, want %+v", got, local)
	}
	if snapshot, ok := negotiator.Snapshot(); ok || snapshot != (ProtocolCompatibility{}) {
		t.Fatalf("initial Snapshot() = (%+v, %v), want zero, false", snapshot, ok)
	}

	want := ProtocolCompatibility{Version: 3, Capabilities: 0b0101}
	got, err := negotiator.Negotiate(ProtocolVersionPolicy{
		Enabled:      true,
		MinVersion:   1,
		MaxVersion:   3,
		Capabilities: 0b0101,
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if got != want {
		t.Fatalf("Negotiate() = %+v, want %+v", got, want)
	}
	if snapshot, ok := negotiator.Snapshot(); !ok || snapshot != want {
		t.Fatalf("Snapshot() = (%+v, %v), want (%+v, true)", snapshot, ok, want)
	}

	_, err = negotiator.Negotiate(ProtocolVersionPolicy{Enabled: true, MinVersion: 5, MaxVersion: 6})
	if !errors.Is(err, ErrNoCompatibleProtocolVersion) {
		t.Fatalf("incompatible Negotiate() error = %v", err)
	}
	if snapshot, ok := negotiator.Snapshot(); !ok || snapshot != want {
		t.Fatalf("Snapshot() after failure = (%+v, %v), want (%+v, true)", snapshot, ok, want)
	}
}

func TestProtocolVersionNegotiatorRejectsInvalidLocalPolicy(t *testing.T) {
	_, err := NewProtocolVersionNegotiator(ProtocolVersionPolicy{
		Enabled:    true,
		MinVersion: 2,
		MaxVersion: 1,
	})
	if !errors.Is(err, ErrInvalidProtocolVersionPolicy) {
		t.Fatalf("NewProtocolVersionNegotiator() error = %v, want ErrInvalidProtocolVersionPolicy", err)
	}

	var negotiator *ProtocolVersionNegotiator
	if _, err := negotiator.Negotiate(ProtocolVersionPolicy{}); !errors.Is(err, ErrInvalidProtocolVersionPolicy) {
		t.Fatalf("nil Negotiate() error = %v, want ErrInvalidProtocolVersionPolicy", err)
	}
	if snapshot, ok := negotiator.Snapshot(); ok || snapshot != (ProtocolCompatibility{}) {
		t.Fatalf("nil Snapshot() = (%+v, %v), want zero, false", snapshot, ok)
	}
}

func TestProtocolVersionNegotiatorConcurrentSnapshots(t *testing.T) {
	local := ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 4, Capabilities: 0b1111}
	negotiator, err := NewProtocolVersionNegotiator(local)
	if err != nil {
		t.Fatalf("NewProtocolVersionNegotiator() error = %v", err)
	}

	peers := []ProtocolVersionPolicy{
		{Enabled: true, MinVersion: 1, MaxVersion: 2, Capabilities: 0b0011},
		{Enabled: true, MinVersion: 2, MaxVersion: 3, Capabilities: 0b0101},
		{Enabled: true, MinVersion: 3, MaxVersion: 4, Capabilities: 0b1001},
	}
	valid := map[ProtocolCompatibility]struct{}{
		{Version: 2, Capabilities: 0b0011}: {},
		{Version: 3, Capabilities: 0b0101}: {},
		{Version: 4, Capabilities: 0b1001}: {},
	}

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		peer := peers[i%len(peers)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 200; attempt++ {
				if _, err := negotiator.Negotiate(peer); err != nil {
					t.Errorf("Negotiate() error = %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 200; attempt++ {
				snapshot, ok := negotiator.Snapshot()
				if !ok {
					continue
				}
				if _, exists := valid[snapshot]; !exists {
					t.Errorf("Snapshot() returned torn or unknown value %+v", snapshot)
					return
				}
			}
		}()
	}
	wg.Wait()

	if snapshot, ok := negotiator.Snapshot(); !ok {
		t.Fatal("Snapshot() remained unset after successful negotiations")
	} else if _, exists := valid[snapshot]; !exists {
		t.Fatalf("final Snapshot() = %+v", snapshot)
	}
}

func FuzzNegotiateProtocolCompatibility(f *testing.F) {
	f.Add(true, int32(1), int32(3), uint64(0b111), true, true, int32(2), int32(4), uint64(0b101), false)
	f.Add(false, int32(0), int32(0), uint64(0), false, true, int32(1), int32(2), uint64(1), true)
	f.Add(true, int32(4), int32(2), uint64(1), false, true, int32(1), int32(3), uint64(1), false)

	f.Fuzz(func(
		t *testing.T,
		localEnabled bool,
		localMin, localMax int32,
		localCapabilities uint64,
		localAcceptLegacy bool,
		peerEnabled bool,
		peerMin, peerMax int32,
		peerCapabilities uint64,
		peerAcceptLegacy bool,
	) {
		local := ProtocolVersionPolicy{
			Enabled:      localEnabled,
			MinVersion:   localMin,
			MaxVersion:   localMax,
			Capabilities: localCapabilities,
			AcceptLegacy: localAcceptLegacy,
		}
		peer := ProtocolVersionPolicy{
			Enabled:      peerEnabled,
			MinVersion:   peerMin,
			MaxVersion:   peerMax,
			Capabilities: peerCapabilities,
			AcceptLegacy: peerAcceptLegacy,
		}

		got, err := NegotiateProtocolCompatibility(local, peer)
		reversed, reversedErr := NegotiateProtocolCompatibility(peer, local)
		if (err == nil) != (reversedErr == nil) {
			t.Fatalf("asymmetric errors: forward=%v reverse=%v", err, reversedErr)
		}
		if err != nil {
			return
		}
		if got != reversed {
			t.Fatalf("asymmetric results: forward=%+v reverse=%+v", got, reversed)
		}
		if got.IsLegacy() {
			if got.Capabilities != 0 {
				t.Fatalf("legacy negotiation exposed capabilities 0x%x", got.Capabilities)
			}
			return
		}
		if !local.Enabled || !peer.Enabled {
			t.Fatalf("positive version %d selected with a disabled policy", got.Version)
		}
		if got.Version < local.MinVersion || got.Version > local.MaxVersion ||
			got.Version < peer.MinVersion || got.Version > peer.MaxVersion {
			t.Fatalf("version %d is outside local [%d,%d] or peer [%d,%d]", got.Version, local.MinVersion, local.MaxVersion, peer.MinVersion, peer.MaxVersion)
		}
		if want := local.Capabilities & peer.Capabilities; got.Capabilities != want {
			t.Fatalf("capabilities = 0x%x, want 0x%x", got.Capabilities, want)
		}
	})
}
