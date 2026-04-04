package zNet

import (
	"context"
	"crypto/rand"
	"sync/atomic"
	"time"

	"github.com/pzqf/zUtil/zMap"
)

type keyRotationData struct {
	currentKey   []byte
	currentKeyID uint32
	previousKeys map[uint32][]byte
}

type KeyRotationManager struct {
	data           atomic.Value
	interval       time.Duration
	maxHistoryKeys int
	ctx            context.Context
	cancel         context.CancelFunc
	lastRotation   atomic.Int64
}

func NewKeyRotationManager(interval time.Duration, maxHistoryKeys int) *KeyRotationManager {
	ctx, cancel := context.WithCancel(context.Background())

	initialKey := generateAESKey()
	data := &keyRotationData{
		currentKey:   initialKey,
		currentKeyID: 1,
		previousKeys: make(map[uint32][]byte),
	}

	krm := &KeyRotationManager{
		interval:       interval,
		maxHistoryKeys: maxHistoryKeys,
		ctx:            ctx,
		cancel:         cancel,
	}
	krm.data.Store(data)
	krm.lastRotation.Store(time.Now().Unix())

	return krm
}

func generateAESKey() []byte {
	key := make([]byte, 16)
	rand.Read(key)
	return key
}

func (krm *KeyRotationManager) Rotate() ([]byte, uint32) {
	data := krm.data.Load().(*keyRotationData)

	newKey := generateAESKey()
	newKeyID := data.currentKeyID + 1

	previousKeys := make(map[uint32][]byte)
	for k, v := range data.previousKeys {
		previousKeys[k] = v
	}
	previousKeys[data.currentKeyID] = data.currentKey

	if len(previousKeys) > krm.maxHistoryKeys {
		minKeyID := uint32(0)
		for k := range previousKeys {
			if minKeyID == 0 || k < minKeyID {
				minKeyID = k
			}
		}
		if minKeyID > 0 {
			delete(previousKeys, minKeyID)
		}
	}

	newData := &keyRotationData{
		currentKey:   newKey,
		currentKeyID: newKeyID,
		previousKeys: previousKeys,
	}
	krm.data.Store(newData)
	krm.lastRotation.Store(time.Now().Unix())

	return newKey, newKeyID
}

func (krm *KeyRotationManager) GetCurrentKey() ([]byte, uint32) {
	data := krm.data.Load().(*keyRotationData)
	return data.currentKey, data.currentKeyID
}

func (krm *KeyRotationManager) GetKeyByID(keyID uint32) ([]byte, bool) {
	data := krm.data.Load().(*keyRotationData)

	if keyID == data.currentKeyID {
		return data.currentKey, true
	}

	if key, exists := data.previousKeys[keyID]; exists {
		return key, true
	}

	return nil, false
}

func (krm *KeyRotationManager) ShouldRotate() bool {
	last := time.Unix(krm.lastRotation.Load(), 0)
	return time.Since(last) >= krm.interval
}

func (krm *KeyRotationManager) StartAutoRotation(getSessionFunc func() []*TcpServerSession) {
	ticker := time.NewTicker(krm.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if krm.ShouldRotate() {
					newKey, newKeyID := krm.Rotate()
					krm.notifySessions(getSessionFunc, newKey, newKeyID)
				}
			case <-krm.ctx.Done():
				return
			}
		}
	}()
}

func (krm *KeyRotationManager) notifySessions(getSessionFunc func() []*TcpServerSession, newKey []byte, newKeyID uint32) {
	sessions := getSessionFunc()
	for _, session := range sessions {
		session.UpdateKey(newKey, newKeyID)
	}
}

func (krm *KeyRotationManager) Stop() {
	krm.cancel()
}

type KeyRotationNotify struct {
	KeyID     uint32
	Timestamp int64
	Nonce     uint64
}

func (k *KeyRotationNotify) Marshal() []byte {
	order := GetByteOrder()
	data := make([]byte, 20)
	order.PutUint32(data[0:4], k.KeyID)
	order.PutUint64(data[4:12], uint64(k.Timestamp))
	order.PutUint64(data[12:20], k.Nonce)
	return data
}

func (k *KeyRotationNotify) Unmarshal(data []byte) bool {
	if len(data) < 20 {
		return false
	}
	order := GetByteOrder()
	k.KeyID = order.Uint32(data[0:4])
	k.Timestamp = int64(order.Uint64(data[4:12]))
	k.Nonce = order.Uint64(data[12:20])
	return true
}

type SequenceManager struct {
	sessions           *zMap.TypedMap[SessionIdType, *SessionSequenceInfo]
	windowSize         uint64
	timestampTolerance int64
}

type SessionSequenceInfo struct {
	lastSeq     atomic.Uint64
	windowStart atomic.Uint64
	window      *zMap.TypedMap[uint64, bool]
}

func NewSequenceManager(windowSize uint64, timestampTolerance int64) *SequenceManager {
	return &SequenceManager{
		sessions:           zMap.NewTypedMap[SessionIdType, *SessionSequenceInfo](),
		windowSize:         windowSize,
		timestampTolerance: timestampTolerance,
	}
}

func (sm *SequenceManager) GetNextSequence() uint64 {
	return 0
}

func (sm *SequenceManager) ValidateSequence(sid SessionIdType, seq uint64, timestamp int64) bool {
	if timestamp != 0 {
		now := time.Now().Unix()
		diff := now - timestamp
		if diff < 0 {
			diff = -diff
		}
		if diff > sm.timestampTolerance {
			return false
		}
	}

	info, exists := sm.sessions.Load(sid)
	if !exists {
		info = &SessionSequenceInfo{
			window: zMap.NewTypedMap[uint64, bool](),
		}
		sm.sessions.Store(sid, info)
		info.lastSeq.Store(seq)
		info.window.Store(seq, true)
		info.windowStart.Store(seq)
		return true
	}

	lastSeq := info.lastSeq.Load()
	if seq <= lastSeq {
		if seq == lastSeq {
			return false
		}
		if lastSeq-seq < sm.windowSize {
			_, exists := info.window.Load(seq)
			return !exists
		}
		return false
	}

	_, exists = info.window.Load(seq)
	if exists {
		return false
	}

	info.window.Store(seq, true)

	if seq-lastSeq > sm.windowSize {
		newWindowStart := seq - sm.windowSize + 1
		info.windowStart.Store(newWindowStart)
		oldWindowStart := info.windowStart.Load()
		for i := oldWindowStart; i < newWindowStart; i++ {
			info.window.Delete(i)
		}
	}

	info.lastSeq.Store(seq)
	return true
}

func (sm *SequenceManager) RemoveSession(sid SessionIdType) {
	sm.sessions.Delete(sid)
}
