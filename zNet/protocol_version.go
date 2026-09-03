package zNet

import (
	"errors"
	"fmt"
	"sync/atomic"
)

const LegacyProtocolVersion int32 = 0

var (
	ErrInvalidProtocolVersionPolicy    = errors.New("invalid protocol version policy")
	ErrNoCompatibleProtocolVersion     = errors.New("no compatible protocol version")
	ErrProtocolCapabilitiesUnavailable = errors.New("required protocol capabilities unavailable")
	ErrProtocolCompatibilityConflict   = errors.New("protocol compatibility already established")
)

// ProtocolVersionPolicy describes one endpoint's version range and capability
// bits without assigning application semantics to either. The zero value is a
// disabled policy and preserves the legacy Version=0 behavior.
type ProtocolVersionPolicy struct {
	Enabled      bool
	MinVersion   int32
	MaxVersion   int32
	Capabilities uint64
	AcceptLegacy bool
}

// Validate rejects ambiguous policies. Enabled policies use strictly positive
// versions; disabled policies must remain the zero value.
func (p ProtocolVersionPolicy) Validate() error {
	if !p.Enabled {
		if p.MinVersion != 0 || p.MaxVersion != 0 || p.Capabilities != 0 || p.AcceptLegacy {
			return fmt.Errorf("%w: disabled policy must use zero values", ErrInvalidProtocolVersionPolicy)
		}
		return nil
	}
	if p.MinVersion <= LegacyProtocolVersion {
		return fmt.Errorf("%w: minimum version %d must be positive", ErrInvalidProtocolVersionPolicy, p.MinVersion)
	}
	if p.MaxVersion < p.MinVersion {
		return fmt.Errorf(
			"%w: maximum version %d is below minimum version %d",
			ErrInvalidProtocolVersionPolicy,
			p.MaxVersion,
			p.MinVersion,
		)
	}
	return nil
}

// ProtocolCompatibility is an immutable value snapshot of a successful
// negotiation. Version=0 always means legacy mode and has no capabilities.
type ProtocolCompatibility struct {
	Version      int32
	Capabilities uint64
}

func (c ProtocolCompatibility) IsLegacy() bool {
	return c.Version == LegacyProtocolVersion
}

func (c ProtocolCompatibility) SupportsAll(required uint64) bool {
	return c.Capabilities&required == required
}

// RequireCapabilities lets the application attach meaning to capability bits
// while zNet provides a stable rejection contract.
func (c ProtocolCompatibility) RequireCapabilities(required uint64) error {
	missing := required &^ c.Capabilities
	if missing == 0 {
		return nil
	}
	return fmt.Errorf("%w: missing mask 0x%016x", ErrProtocolCapabilitiesUnavailable, missing)
}

// NegotiateProtocolCompatibility selects the highest common positive version
// and intersects capability bits. A disabled policy represents a legacy peer;
// an enabled peer must opt in through AcceptLegacy before Version=0 is chosen.
func NegotiateProtocolCompatibility(local, peer ProtocolVersionPolicy) (ProtocolCompatibility, error) {
	if err := local.Validate(); err != nil {
		return ProtocolCompatibility{}, fmt.Errorf("local: %w", err)
	}
	if err := peer.Validate(); err != nil {
		return ProtocolCompatibility{}, fmt.Errorf("peer: %w", err)
	}

	if !local.Enabled || !peer.Enabled {
		switch {
		case !local.Enabled && !peer.Enabled:
			return ProtocolCompatibility{Version: LegacyProtocolVersion}, nil
		case local.Enabled && local.AcceptLegacy:
			return ProtocolCompatibility{Version: LegacyProtocolVersion}, nil
		case peer.Enabled && peer.AcceptLegacy:
			return ProtocolCompatibility{Version: LegacyProtocolVersion}, nil
		default:
			return ProtocolCompatibility{}, fmt.Errorf(
				"%w: legacy version %d is not accepted",
				ErrNoCompatibleProtocolVersion,
				LegacyProtocolVersion,
			)
		}
	}

	minimum := max(local.MinVersion, peer.MinVersion)
	maximum := min(local.MaxVersion, peer.MaxVersion)
	if minimum > maximum {
		return ProtocolCompatibility{}, fmt.Errorf(
			"%w: local [%d,%d], peer [%d,%d]",
			ErrNoCompatibleProtocolVersion,
			local.MinVersion,
			local.MaxVersion,
			peer.MinVersion,
			peer.MaxVersion,
		)
	}

	return ProtocolCompatibility{
		Version:      maximum,
		Capabilities: local.Capabilities & peer.Capabilities,
	}, nil
}

// ProtocolVersionNegotiator owns an immutable local policy and publishes the
// first successful result as one atomic snapshot. Repeating the same result is
// idempotent; a conflicting result is rejected to prevent in-session downgrade.
type ProtocolVersionNegotiator struct {
	local   ProtocolVersionPolicy
	current atomic.Pointer[ProtocolCompatibility]
}

func NewProtocolVersionNegotiator(local ProtocolVersionPolicy) (*ProtocolVersionNegotiator, error) {
	if err := local.Validate(); err != nil {
		return nil, err
	}
	return &ProtocolVersionNegotiator{local: local}, nil
}

func (n *ProtocolVersionNegotiator) LocalPolicy() ProtocolVersionPolicy {
	if n == nil {
		return ProtocolVersionPolicy{}
	}
	return n.local
}

func (n *ProtocolVersionNegotiator) Negotiate(peer ProtocolVersionPolicy) (ProtocolCompatibility, error) {
	if n == nil {
		return ProtocolCompatibility{}, fmt.Errorf("%w: nil negotiator", ErrInvalidProtocolVersionPolicy)
	}
	compatibility, err := NegotiateProtocolCompatibility(n.local, peer)
	if err != nil {
		return ProtocolCompatibility{}, err
	}
	snapshot := compatibility
	if n.current.CompareAndSwap(nil, &snapshot) {
		return compatibility, nil
	}
	current := n.current.Load()
	if current != nil && *current == compatibility {
		return *current, nil
	}
	return ProtocolCompatibility{}, fmt.Errorf(
		"%w: current version=%d capabilities=0x%016x, candidate version=%d capabilities=0x%016x",
		ErrProtocolCompatibilityConflict,
		current.Version,
		current.Capabilities,
		compatibility.Version,
		compatibility.Capabilities,
	)
}

// Snapshot returns a complete point-in-time copy and whether any negotiation
// has succeeded. Concurrent calls with Negotiate never observe torn fields.
func (n *ProtocolVersionNegotiator) Snapshot() (ProtocolCompatibility, bool) {
	if n == nil {
		return ProtocolCompatibility{}, false
	}
	snapshot := n.current.Load()
	if snapshot == nil {
		return ProtocolCompatibility{}, false
	}
	return *snapshot, true
}
