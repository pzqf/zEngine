package zNet

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// ProtocolNegotiationProtoId deliberately uses a positive value so older
	// zNet versions accept and dispatch it as an unknown optional message rather
	// than rejecting the connection during header validation.
	ProtocolNegotiationProtoId ProtoIdType = 1<<31 - 1

	protocolNegotiationMagic            uint32 = 0x5A4E4554 // ZNET
	protocolNegotiationFormatVersion    uint16 = 1
	protocolNegotiationFrameSize               = 24
	protocolNegotiationFlagAcceptLegacy        = uint16(1 << 0)
	protocolNegotiationKnownFlags              = protocolNegotiationFlagAcceptLegacy
)

var (
	ErrInvalidProtocolNegotiationFrame     = errors.New("invalid protocol negotiation frame")
	ErrProtocolCompatibilityNotEstablished = errors.New("protocol compatibility not established")
	ErrProtocolPacketVersionMismatch       = errors.New("protocol packet version mismatch")
)

// MarshalProtocolNegotiation encodes an enabled policy in one fixed-size,
// byte-order-independent control payload. Disabled endpoints send no frame.
func MarshalProtocolNegotiation(policy ProtocolVersionPolicy) ([]byte, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return nil, fmt.Errorf("%w: disabled policy", ErrInvalidProtocolNegotiationFrame)
	}

	flags := uint16(0)
	if policy.AcceptLegacy {
		flags |= protocolNegotiationFlagAcceptLegacy
	}
	frame := make([]byte, protocolNegotiationFrameSize)
	binary.BigEndian.PutUint32(frame[0:4], protocolNegotiationMagic)
	binary.BigEndian.PutUint16(frame[4:6], protocolNegotiationFormatVersion)
	binary.BigEndian.PutUint16(frame[6:8], flags)
	binary.BigEndian.PutUint32(frame[8:12], uint32(policy.MinVersion))
	binary.BigEndian.PutUint32(frame[12:16], uint32(policy.MaxVersion))
	binary.BigEndian.PutUint64(frame[16:24], policy.Capabilities)
	return frame, nil
}

// UnmarshalProtocolNegotiation decodes and validates one negotiation frame.
func UnmarshalProtocolNegotiation(frame []byte) (ProtocolVersionPolicy, error) {
	if len(frame) != protocolNegotiationFrameSize {
		return ProtocolVersionPolicy{}, fmt.Errorf(
			"%w: size %d, want %d",
			ErrInvalidProtocolNegotiationFrame,
			len(frame),
			protocolNegotiationFrameSize,
		)
	}
	if magic := binary.BigEndian.Uint32(frame[0:4]); magic != protocolNegotiationMagic {
		return ProtocolVersionPolicy{}, fmt.Errorf("%w: magic 0x%08x", ErrInvalidProtocolNegotiationFrame, magic)
	}
	if version := binary.BigEndian.Uint16(frame[4:6]); version != protocolNegotiationFormatVersion {
		return ProtocolVersionPolicy{}, fmt.Errorf("%w: format version %d", ErrInvalidProtocolNegotiationFrame, version)
	}
	flags := binary.BigEndian.Uint16(frame[6:8])
	if unknown := flags &^ protocolNegotiationKnownFlags; unknown != 0 {
		return ProtocolVersionPolicy{}, fmt.Errorf("%w: unknown flags 0x%04x", ErrInvalidProtocolNegotiationFrame, unknown)
	}

	policy := ProtocolVersionPolicy{
		Enabled:      true,
		MinVersion:   int32(binary.BigEndian.Uint32(frame[8:12])),
		MaxVersion:   int32(binary.BigEndian.Uint32(frame[12:16])),
		Capabilities: binary.BigEndian.Uint64(frame[16:24]),
		AcceptLegacy: flags&protocolNegotiationFlagAcceptLegacy != 0,
	}
	if err := policy.Validate(); err != nil {
		return ProtocolVersionPolicy{}, fmt.Errorf("%w: %v", ErrInvalidProtocolNegotiationFrame, err)
	}
	return policy, nil
}

// OutboundPacketVersion returns the version to stamp on a packet. Disabled
// policies preserve Version=0. Enabled sessions may send only the negotiation
// frame until compatibility has been established.
func (n *ProtocolVersionNegotiator) OutboundPacketVersion(protoID ProtoIdType) (int32, error) {
	if n == nil {
		return LegacyProtocolVersion, fmt.Errorf("%w: nil negotiator", ErrInvalidProtocolVersionPolicy)
	}
	if !n.local.Enabled {
		return LegacyProtocolVersion, nil
	}
	if protoID == ProtocolNegotiationProtoId {
		return n.local.MaxVersion, nil
	}
	compatibility, ok := n.Snapshot()
	if !ok {
		return LegacyProtocolVersion, ErrProtocolCompatibilityNotEstablished
	}
	return compatibility.Version, nil
}

// ValidateInboundPacketVersion enforces the frozen session version before an
// application payload is decoded. A Version=0 application packet can establish
// legacy compatibility only when the enabled local policy explicitly permits it.
func (n *ProtocolVersionNegotiator) ValidateInboundPacketVersion(protoID ProtoIdType, version int32) error {
	if n == nil {
		return fmt.Errorf("%w: nil negotiator", ErrInvalidProtocolVersionPolicy)
	}
	if !n.local.Enabled {
		return nil
	}
	if protoID == ProtocolNegotiationProtoId {
		if version <= LegacyProtocolVersion {
			return fmt.Errorf("%w: negotiation version %d", ErrProtocolPacketVersionMismatch, version)
		}
		return nil
	}

	compatibility, ok := n.Snapshot()
	if !ok {
		if version != LegacyProtocolVersion {
			return fmt.Errorf(
				"%w: packet version %d before negotiation",
				ErrProtocolCompatibilityNotEstablished,
				version,
			)
		}
		_, err := n.Negotiate(ProtocolVersionPolicy{})
		return err
	}
	if version != compatibility.Version {
		return fmt.Errorf(
			"%w: got %d, want %d",
			ErrProtocolPacketVersionMismatch,
			version,
			compatibility.Version,
		)
	}
	return nil
}

// AcceptProtocolNegotiation validates that the header advertises the peer's
// maximum version, then freezes the resulting compatibility snapshot.
func (n *ProtocolVersionNegotiator) AcceptProtocolNegotiation(packetVersion int32, frame []byte) (ProtocolCompatibility, error) {
	if n == nil {
		return ProtocolCompatibility{}, fmt.Errorf("%w: nil negotiator", ErrInvalidProtocolVersionPolicy)
	}
	if !n.local.Enabled {
		return ProtocolCompatibility{}, ErrProtocolCompatibilityNotEstablished
	}
	peer, err := UnmarshalProtocolNegotiation(frame)
	if err != nil {
		return ProtocolCompatibility{}, err
	}
	if packetVersion != peer.MaxVersion {
		return ProtocolCompatibility{}, fmt.Errorf(
			"%w: negotiation header %d, peer maximum %d",
			ErrProtocolPacketVersionMismatch,
			packetVersion,
			peer.MaxVersion,
		)
	}
	return n.Negotiate(peer)
}
