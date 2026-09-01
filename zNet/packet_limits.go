package zNet

// normalizePacketSizeLimits resolves the compatibility field and makes every
// endpoint bounded. An explicit MaxWirePacketSize wins over the legacy field;
// zero and negative values use the 1 MiB defaults.
func normalizePacketSizeLimits(maxWire, legacyWire, maxDecoded *int32) {
	wire := *maxWire
	if wire <= 0 {
		wire = *legacyWire
	}
	if wire <= 0 {
		wire = DefaultWirePacketSize
	}
	decoded := *maxDecoded
	if decoded <= 0 {
		decoded = DefaultDecodedPacketSize
	}

	*maxWire = wire
	*legacyWire = wire
	*maxDecoded = decoded
}

func resolveWirePacketSize(maxWire, legacyWire int32) int32 {
	decoded := int32(0)
	normalizePacketSizeLimits(&maxWire, &legacyWire, &decoded)
	return maxWire
}

func resolveDecodedPacketSize(maxDecoded int32) int32 {
	wire := int32(0)
	legacy := int32(0)
	normalizePacketSizeLimits(&wire, &legacy, &maxDecoded)
	return maxDecoded
}
