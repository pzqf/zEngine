package zNet

import (
	"errors"
	"fmt"

	"github.com/golang/snappy"
	"github.com/pzqf/zUtil/zCrypto"
)

var (
	ErrPacketDecodedTooLarge = errors.New("decoded packet data exceeds limit")
	ErrPacketDecrypt         = errors.New("packet decrypt failed")
	ErrPacketDecompress      = errors.New("packet decompress failed")
)

const aesGCMWireOverhead = 12 + 16 // nonce + authentication tag in zCrypto's GCM wire format

func decodedPacketSizeError(size int, maxSize int32) error {
	return fmt.Errorf("%w: got %d, max %d", ErrPacketDecodedTooLarge, size, maxSize)
}

func validateDecodedPacketSize(size int, maxSize int32) error {
	maxSize = resolveDecodedPacketSize(maxSize)
	if size > int(maxSize) {
		return decodedPacketSizeError(size, maxSize)
	}
	return nil
}

// decodePacketPayload applies endpoint resource limits to each allocation stage.
// It mutates packet only after all decrypt/decompress work has succeeded.
func decodePacketPayload(packet *NetPacket, decryptKey []byte, encrypted bool, maxDecodedPacketSize int32) error {
	if packet == nil {
		return fmt.Errorf("%w: nil packet", ErrPacketDataSize)
	}

	data := packet.Data
	if encrypted {
		if len(decryptKey) == 0 {
			return fmt.Errorf("%w: missing key", ErrPacketDecrypt)
		}
		if len(data) < aesGCMWireOverhead {
			return fmt.Errorf("%w: ciphertext length %d is smaller than GCM overhead %d",
				ErrPacketDecrypt, len(data), aesGCMWireOverhead)
		}
		plaintextSize := len(data) - aesGCMWireOverhead
		if err := validateDecodedPacketSize(plaintextSize, maxDecodedPacketSize); err != nil {
			return err
		}
		decrypted, err := zCrypto.AESDecrypt(data, decryptKey, nil, zCrypto.AESModeGCM)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPacketDecrypt, err)
		}
		if len(decrypted) != plaintextSize {
			return fmt.Errorf("%w: got plaintext length %d, want %d", ErrPacketDecrypt, len(decrypted), plaintextSize)
		}
		if err := validateDecodedPacketSize(len(decrypted), maxDecodedPacketSize); err != nil {
			return err
		}
		data = decrypted
	}

	if packet.IsCompressed == CompressionSnappy {
		decodedSize, err := snappy.DecodedLen(data)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPacketDecompress, err)
		}
		if err := validateDecodedPacketSize(decodedSize, maxDecodedPacketSize); err != nil {
			return err
		}
		decoded, err := snappy.Decode(make([]byte, 0, decodedSize), data)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPacketDecompress, err)
		}
		if len(decoded) != decodedSize {
			return fmt.Errorf("%w: got decoded length %d, want %d", ErrPacketDecompress, len(decoded), decodedSize)
		}
		if err := validateDecodedPacketSize(len(decoded), maxDecodedPacketSize); err != nil {
			return err
		}
		data = decoded
	} else if err := validateDecodedPacketSize(len(data), maxDecodedPacketSize); err != nil {
		return err
	}

	packet.Data = data
	packet.DataSize = int32(len(data))
	return nil
}
