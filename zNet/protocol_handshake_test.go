package zNet

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestProtocolNegotiationFrameRoundTrip(t *testing.T) {
	want := ProtocolVersionPolicy{
		Enabled:      true,
		MinVersion:   2,
		MaxVersion:   4,
		Capabilities: 0xA5A55A5AF0F00F0F,
		AcceptLegacy: true,
	}
	frame, err := MarshalProtocolNegotiation(want)
	if err != nil {
		t.Fatalf("MarshalProtocolNegotiation() error = %v", err)
	}
	if len(frame) != protocolNegotiationFrameSize {
		t.Fatalf("frame size = %d, want %d", len(frame), protocolNegotiationFrameSize)
	}
	got, err := UnmarshalProtocolNegotiation(frame)
	if err != nil {
		t.Fatalf("UnmarshalProtocolNegotiation() error = %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestProtocolNegotiationFrameRejectsInvalidInput(t *testing.T) {
	valid, err := MarshalProtocolNegotiation(ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 2})
	if err != nil {
		t.Fatalf("MarshalProtocolNegotiation() error = %v", err)
	}

	tests := []struct {
		name  string
		frame []byte
	}{
		{name: "short", frame: valid[:len(valid)-1]},
		{name: "long", frame: append(append([]byte(nil), valid...), 0)},
		{name: "bad magic", frame: mutateProtocolFrame(valid, func(frame []byte) { binary.BigEndian.PutUint32(frame[0:4], 0) })},
		{name: "bad format", frame: mutateProtocolFrame(valid, func(frame []byte) { binary.BigEndian.PutUint16(frame[4:6], 2) })},
		{name: "unknown flags", frame: mutateProtocolFrame(valid, func(frame []byte) { binary.BigEndian.PutUint16(frame[6:8], 2) })},
		{name: "zero minimum", frame: mutateProtocolFrame(valid, func(frame []byte) { binary.BigEndian.PutUint32(frame[8:12], 0) })},
		{name: "reversed range", frame: mutateProtocolFrame(valid, func(frame []byte) { binary.BigEndian.PutUint32(frame[8:12], 3) })},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := UnmarshalProtocolNegotiation(test.frame); !errors.Is(err, ErrInvalidProtocolNegotiationFrame) {
				t.Fatalf("UnmarshalProtocolNegotiation() error = %v, want ErrInvalidProtocolNegotiationFrame", err)
			}
		})
	}

	if _, err := MarshalProtocolNegotiation(ProtocolVersionPolicy{}); !errors.Is(err, ErrInvalidProtocolNegotiationFrame) {
		t.Fatalf("MarshalProtocolNegotiation(disabled) error = %v, want ErrInvalidProtocolNegotiationFrame", err)
	}
}

func TestProtocolNegotiatorPacketVersions(t *testing.T) {
	disabled, err := NewProtocolVersionNegotiator(ProtocolVersionPolicy{})
	if err != nil {
		t.Fatalf("NewProtocolVersionNegotiator(disabled) error = %v", err)
	}
	if got, err := disabled.OutboundPacketVersion(123); err != nil || got != LegacyProtocolVersion {
		t.Fatalf("disabled OutboundPacketVersion() = (%d, %v)", got, err)
	}
	if err := disabled.ValidateInboundPacketVersion(123, -99); err != nil {
		t.Fatalf("disabled ValidateInboundPacketVersion() error = %v", err)
	}

	local := ProtocolVersionPolicy{Enabled: true, MinVersion: 2, MaxVersion: 4, Capabilities: 0b1111, AcceptLegacy: true}
	negotiator, err := NewProtocolVersionNegotiator(local)
	if err != nil {
		t.Fatalf("NewProtocolVersionNegotiator() error = %v", err)
	}
	if got, err := negotiator.OutboundPacketVersion(ProtocolNegotiationProtoId); err != nil || got != 4 {
		t.Fatalf("negotiation OutboundPacketVersion() = (%d, %v), want (4, nil)", got, err)
	}
	if _, err := negotiator.OutboundPacketVersion(123); !errors.Is(err, ErrProtocolCompatibilityNotEstablished) {
		t.Fatalf("application OutboundPacketVersion() error = %v, want ErrProtocolCompatibilityNotEstablished", err)
	}
	if err := negotiator.ValidateInboundPacketVersion(ProtocolNegotiationProtoId, 0); !errors.Is(err, ErrProtocolPacketVersionMismatch) {
		t.Fatalf("negotiation ValidateInboundPacketVersion(0) error = %v", err)
	}
	if err := negotiator.ValidateInboundPacketVersion(123, 3); !errors.Is(err, ErrProtocolCompatibilityNotEstablished) {
		t.Fatalf("pre-negotiation positive packet error = %v", err)
	}

	peer := ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 3, Capabilities: 0b0101}
	frame, err := MarshalProtocolNegotiation(peer)
	if err != nil {
		t.Fatalf("MarshalProtocolNegotiation() error = %v", err)
	}
	compatibility, err := negotiator.AcceptProtocolNegotiation(peer.MaxVersion, frame)
	if err != nil {
		t.Fatalf("AcceptProtocolNegotiation() error = %v", err)
	}
	want := ProtocolCompatibility{Version: 3, Capabilities: 0b0101}
	if compatibility != want {
		t.Fatalf("AcceptProtocolNegotiation() = %+v, want %+v", compatibility, want)
	}
	if got, err := negotiator.OutboundPacketVersion(123); err != nil || got != want.Version {
		t.Fatalf("established OutboundPacketVersion() = (%d, %v), want (%d, nil)", got, err, want.Version)
	}
	if err := negotiator.ValidateInboundPacketVersion(123, want.Version); err != nil {
		t.Fatalf("established ValidateInboundPacketVersion() error = %v", err)
	}
	if err := negotiator.ValidateInboundPacketVersion(123, want.Version-1); !errors.Is(err, ErrProtocolPacketVersionMismatch) {
		t.Fatalf("mismatched packet error = %v, want ErrProtocolPacketVersionMismatch", err)
	}
}

func TestProtocolNegotiatorLegacyPacketEstablishesCompatibility(t *testing.T) {
	accepting, err := NewProtocolVersionNegotiator(ProtocolVersionPolicy{
		Enabled: true, MinVersion: 1, MaxVersion: 2, AcceptLegacy: true,
	})
	if err != nil {
		t.Fatalf("NewProtocolVersionNegotiator() error = %v", err)
	}
	if err := accepting.ValidateInboundPacketVersion(123, LegacyProtocolVersion); err != nil {
		t.Fatalf("ValidateInboundPacketVersion(legacy) error = %v", err)
	}
	if snapshot, ok := accepting.Snapshot(); !ok || !snapshot.IsLegacy() {
		t.Fatalf("Snapshot() = (%+v, %v), want established legacy", snapshot, ok)
	}

	rejecting, err := NewProtocolVersionNegotiator(ProtocolVersionPolicy{
		Enabled: true, MinVersion: 1, MaxVersion: 2,
	})
	if err != nil {
		t.Fatalf("NewProtocolVersionNegotiator() error = %v", err)
	}
	if err := rejecting.ValidateInboundPacketVersion(123, LegacyProtocolVersion); !errors.Is(err, ErrNoCompatibleProtocolVersion) {
		t.Fatalf("rejecting legacy error = %v, want ErrNoCompatibleProtocolVersion", err)
	}
	if _, ok := rejecting.Snapshot(); ok {
		t.Fatal("rejected legacy packet established compatibility")
	}
}

func TestAcceptProtocolNegotiationRequiresMatchingHeader(t *testing.T) {
	negotiator, err := NewProtocolVersionNegotiator(ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 3})
	if err != nil {
		t.Fatalf("NewProtocolVersionNegotiator() error = %v", err)
	}
	frame, err := MarshalProtocolNegotiation(ProtocolVersionPolicy{Enabled: true, MinVersion: 1, MaxVersion: 2})
	if err != nil {
		t.Fatalf("MarshalProtocolNegotiation() error = %v", err)
	}
	if _, err := negotiator.AcceptProtocolNegotiation(1, frame); !errors.Is(err, ErrProtocolPacketVersionMismatch) {
		t.Fatalf("AcceptProtocolNegotiation() error = %v, want ErrProtocolPacketVersionMismatch", err)
	}
	if _, ok := negotiator.Snapshot(); ok {
		t.Fatal("mismatched negotiation header established compatibility")
	}
}

func mutateProtocolFrame(frame []byte, mutate func([]byte)) []byte {
	clone := append([]byte(nil), frame...)
	mutate(clone)
	return clone
}

func FuzzUnmarshalProtocolNegotiation(f *testing.F) {
	valid, err := MarshalProtocolNegotiation(ProtocolVersionPolicy{
		Enabled: true, MinVersion: 1, MaxVersion: 2, Capabilities: 3, AcceptLegacy: true,
	})
	if err != nil {
		f.Fatalf("MarshalProtocolNegotiation() error = %v", err)
	}
	f.Add(valid)
	f.Add([]byte{})
	f.Add(make([]byte, protocolNegotiationFrameSize))

	f.Fuzz(func(t *testing.T, frame []byte) {
		policy, err := UnmarshalProtocolNegotiation(frame)
		if err != nil {
			return
		}
		if err := policy.Validate(); err != nil {
			t.Fatalf("decoded policy is invalid: %+v: %v", policy, err)
		}
		if !policy.Enabled {
			t.Fatalf("decoded policy is disabled: %+v", policy)
		}
		roundTrip, err := MarshalProtocolNegotiation(policy)
		if err != nil {
			t.Fatalf("MarshalProtocolNegotiation(decoded) error = %v", err)
		}
		if got, err := UnmarshalProtocolNegotiation(roundTrip); err != nil || got != policy {
			t.Fatalf("round trip = (%+v, %v), want (%+v, nil)", got, err, policy)
		}
	})
}
