package zNet

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/pzqf/zUtil/zCrypto"
)

type UdpServerSession struct {
	addr              *net.UDPAddr
	sid               SessionIdType
	sendQueue         *outboundQueue
	receiveChan       chan *NetPacket
	wg                sync.WaitGroup
	lastHeartBeat     time.Time
	ctxCancel         context.CancelFunc
	onClose           UdpCloseCallBackFunc
	closeOnce         sync.Once
	aesKey            []byte
	dhExchange        *DHKeyExchange
	svr               *UdpServer
	obj               interface{}
	packetCodec       PacketCodec
	invalidPacketLogs packetErrorLogLimiter
}

type UdpCloseCallBackFunc func(c *UdpServerSession)

func (s *UdpServerSession) triggerOnClose() {
	s.closeOnce.Do(func() {
		if s.onClose != nil {
			s.onClose(s)
		}
	})
}

func NewUdpServerSession(svr *UdpServer, addr *net.UDPAddr, sid SessionIdType, closeCallBack UdpCloseCallBackFunc, aesKey []byte, dhExchange *DHKeyExchange) *UdpServerSession {
	newSession := UdpServerSession{
		addr:          addr,
		sid:           sid,
		sendQueue:     newOutboundQueue(svr.config.ChanSize, backpressureMetrics(svr.metrics)),
		receiveChan:   make(chan *NetPacket, svr.config.ChanSize),
		lastHeartBeat: time.Now(),
		onClose:       closeCallBack,
		aesKey:        aesKey,
		dhExchange:    dhExchange,
		svr:           svr,
		packetCodec:   svr.packetCodec,
	}
	return &newSession
}

func (s *UdpServerSession) Start() {
	if s.addr == nil {
		return
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctxCancel = ctxCancel

	// NET-4: wg.Add 必须在 go 之前（否则 Close 的 wg.Wait 可能计数 0 提前返回，与 goroutine 竞争）。
	s.wg.Add(1)
	go s.process(ctx)
	if s.svr.config.HeartbeatDuration > 0 {
		s.wg.Add(1)
		go s.heartbeatCheck(ctx)
	}
}

func (s *UdpServerSession) Close() {
	s.sendQueue.Close(ErrSessionClosed)
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	s.wg.Wait()
}

func (s *UdpServerSession) handlePacket(data []byte) {
	// 处理ECDH密钥交换
	if s.dhExchange != nil && s.aesKey == nil {
		if len(data) == 64 {
			// 收到客户端的公钥，执行密钥交换
			aesKey, err := s.dhExchange.ComputeSharedSecret(data)
			if err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Failed to compute shared secret for UDP session: %v", err)
				}
			} else {
				s.aesKey = aesKey
				if s.svr.logger != nil {
					s.svr.logger.Info("ECDH key exchange successful for UDP session, key length: %d", len(aesKey))
				}
				// 发送服务器的公钥给客户端
				serverPublicKey := s.dhExchange.GetPublicKey()
				if _, err := s.svr.listener.WriteToUDP(serverPublicKey, s.addr); err != nil {
					if s.svr.logger != nil {
						s.svr.logger.Error("Failed to send server public key to UDP client: %v", err)
					}
				}
			}
			// 密钥交换完成，清理资源
			s.dhExchange = nil
			return
		} else {
			// 第一次收到数据包，发送服务器的公钥给客户端
			serverPublicKey := s.dhExchange.GetPublicKey()
			if _, err := s.svr.listener.WriteToUDP(serverPublicKey, s.addr); err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Failed to send server public key to UDP client: %v", err)
				}
			}
			if s.svr.logger != nil {
				s.svr.logger.Info("Sent server public key to UDP client for key exchange")
			}
			return
		}
	}

	// UDP 是消息边界传输：非法帧只丢当前 datagram，不拆会话。
	netPacket, err := s.packetCodec.DecodeFrame(data, s.svr.config.MaxWirePacketSize)
	if err != nil {
		recordPacketDecodeError(s.svr.metrics, err)
		if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
			s.svr.logger.Error("Invalid UDP packet: %v", err)
		}
		return
	}

	if netPacket.ProtoId != HeartbeatProtoId {
		s.receiveChan <- &netPacket
	}

	s.heartbeatUpdate()
}

// dispatchSafely 调用 dispatcher 处理单个包，并隔离其 error 与 panic——绝不因单包问题拆掉会话（NET-4）。
func (s *UdpServerSession) dispatchSafely(receivePacket *NetPacket) {
	defer func() {
		if r := recover(); r != nil && s.svr.logger != nil {
			s.svr.logger.Error("Dispatcher panic: %v, ProtoId: %d", r, receivePacket.ProtoId)
		}
	}()
	if err := s.svr.dispatcher(s, receivePacket); err != nil && s.svr.logger != nil {
		s.svr.logger.Error("Dispatcher error: %v, ProtoId: %d", err, receivePacket.ProtoId)
	}
}

func (s *UdpServerSession) processReceivedPacket(receivePacket *NetPacket) {
	encrypted := len(receivePacket.Data) > 0 && s.aesKey != nil
	if err := decodePacketPayload(receivePacket, s.aesKey, encrypted, s.svr.config.MaxDecodedPacketSize); err != nil {
		recordPacketDecodeError(s.svr.metrics, err)
		if s.svr.logger != nil && s.invalidPacketLogs.Allow(time.Now()) {
			s.svr.logger.Error("Decode UDP payload error: %v, sid:%d", err, s.sid)
		}
		return
	}
	if s.svr.dispatcher != nil {
		s.dispatchSafely(receivePacket)
	}
}

func (s *UdpServerSession) process(ctx context.Context) {
	defer s.wg.Done()
	defer func() {
		s.triggerOnClose()
		if err := recover(); err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("process panic:%v, sid:%d, closed", err, s.sid)
			}
		}
	}()
	running := true
	for {
		select {
		case receivePacket := <-s.receiveChan:
			s.processReceivedPacket(receivePacket)

		case <-s.sendQueue.Ready():
			sendPacket, ok := s.sendQueue.TryDequeue()
			if !ok {
				continue
			}
			_, err := s.sendOutbound(sendPacket)
			if err != nil {
				if s.svr.logger != nil {
					s.svr.logger.Error("Send UDP packet error:%v, ProtoId:%d", err, sendPacket.packet.ProtoId)
				}
			}
		case <-ctx.Done():
			for {
				if len(s.receiveChan) > 0 {
					receivePacket := <-s.receiveChan
					s.processReceivedPacket(receivePacket)

					continue
				}
				break
			}
			s.sendQueue.Close(ErrSessionClosed)
			for {
				if sendPacket, ok := s.sendQueue.TryDequeue(); ok {
					_, err := s.sendOutbound(sendPacket)
					if err != nil {
						if s.svr.logger != nil {
							s.svr.logger.Error("Send UDP packet error:%v, ProtoId:%d", err, sendPacket.packet.ProtoId)
						}
						break
					}
					continue
				}
				break
			}

			running = false
		}
		if !running {
			break
		}
	}

	s.triggerOnClose()
}

func (s *UdpServerSession) Send(protoId ProtoIdType, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultSendTimeout)
	defer cancel()
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

func (s *UdpServerSession) SendContext(ctx context.Context, protoId ProtoIdType, data []byte) error {
	return s.SendWithOptions(ctx, protoId, data, SendOptions{Class: ReliableCommand})
}

func (s *UdpServerSession) SendWithOptions(ctx context.Context, protoId ProtoIdType, data []byte, options SendOptions) error {
	ctx, cancel := normalizeSendContext(ctx, DefaultSendTimeout)
	defer cancel()
	if options.Class == LatestFrame && options.CoalesceKey == 0 {
		options.CoalesceKey = uint64(uint32(protoId))
	}
	netPacket, err := s.buildOutboundPacket(protoId, data)
	if err != nil {
		return err
	}
	return s.sendQueue.Enqueue(ctx, &outboundPacket{packet: netPacket, deadline: outboundDeadline(ctx)}, options)
}

func (s *UdpServerSession) buildOutboundPacket(protoId ProtoIdType, data []byte) (*NetPacket, error) {
	netPacket := NetPacket{
		ProtoId: protoId,
	}
	if s.aesKey != nil {
		encryptedData, err := zCrypto.AESEncrypt(data, s.aesKey, nil, zCrypto.AESModeGCM)
		if err != nil {
			if s.svr.logger != nil {
				s.svr.logger.Error("AESEncrypt error: %v", err)
			}
			return nil, err
		}
		netPacket.Data = encryptedData
	} else {
		netPacket.Data = append([]byte(nil), data...)
	}
	netPacket.DataSize = int32(len(netPacket.Data))
	if err := ValidatePacketHeader(&netPacket, s.svr.config.MaxWirePacketSize); err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Send UDP packet illegal: %v", err)
		}
		return nil, err
	}
	return &netPacket, nil
}

func (s *UdpServerSession) sendOutbound(outbound *outboundPacket) (int, error) {
	if outbound == nil || outbound.packet == nil {
		return 0, errors.New("nil outbound packet")
	}
	s.svr.writeMu.Lock()
	defer s.svr.writeMu.Unlock()
	if !outbound.deadline.IsZero() {
		if err := s.svr.listener.SetWriteDeadline(outbound.deadline); err != nil {
			return 0, err
		}
		defer s.svr.listener.SetWriteDeadline(time.Time{})
	}
	return s.send(outbound.packet)
}

func (s *UdpServerSession) send(netPacket *NetPacket) (int, error) {
	data := s.packetCodec.Marshal(netPacket)
	n, err := s.svr.listener.WriteToUDP(data, s.addr)
	if err != nil {
		if s.svr.logger != nil {
			s.svr.logger.Error("Failed to write to UDP: %v, ProtoId: %d", err, netPacket.ProtoId)
		}
		return 0, err
	}
	return n, nil
}

func (s *UdpServerSession) heartbeatUpdate() {
	s.lastHeartBeat = time.Now()
}

func (s *UdpServerSession) heartbeatCheck(ctx context.Context) {
	defer s.wg.Done()
	duration := time.Second * time.Duration(s.svr.config.HeartbeatDuration)
	breakDuration := time.Second * time.Duration(s.svr.config.HeartbeatDuration*2)
	for {
		select {
		case <-time.After(duration):
			if time.Since(s.lastHeartBeat) > breakDuration {
				s.ctxCancel()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *UdpServerSession) GetSid() SessionIdType {
	return s.sid
}

func (s *UdpServerSession) GetObj() interface{} {
	return s.obj
}

func (s *UdpServerSession) SetObj(obj interface{}) {
	s.obj = obj
}

// GetClientIP 获取客户端IP地址
func (s *UdpServerSession) GetClientIP() string {
	if s.addr != nil {
		return s.addr.IP.String()
	}
	return ""
}

func (s *UdpServerSession) GetAddr() *net.UDPAddr {
	return s.addr
}
