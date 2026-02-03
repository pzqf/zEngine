package zNet

import (
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"math/big"
)

// DHKeyExchange 处理Diffie-Hellman密钥交换

type DHKeyExchange struct {
	curve      elliptic.Curve
	privateKey []byte
	publicKeyX *big.Int
	publicKeyY *big.Int
}

// NewDHKeyExchange 创建新的DH密钥交换实例
func NewDHKeyExchange() (*DHKeyExchange, error) {
	// 使用P-256椭圆曲线
	curve := elliptic.P256()

	dh := &DHKeyExchange{
		curve: curve,
	}

	// 生成密钥对
	privateKey, publicKeyX, publicKeyY, err := elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}

	dh.privateKey = privateKey
	dh.publicKeyX = publicKeyX
	dh.publicKeyY = publicKeyY

	return dh, nil
}

// GetPublicKey 获取公钥（需要发送给对方）
func (d *DHKeyExchange) GetPublicKey() []byte {
	// 序列化公钥：先X坐标，后Y坐标
	xBytes := d.publicKeyX.Bytes()
	yBytes := d.publicKeyY.Bytes()

	// 确保X和Y坐标都是32字节
	xBytes32 := make([]byte, 32)
	yBytes32 := make([]byte, 32)

	copy(xBytes32[32-len(xBytes):], xBytes)
	copy(yBytes32[32-len(yBytes):], yBytes)

	// 合并X和Y坐标
	publicKey := append(xBytes32, yBytes32...)
	return publicKey
}

// ComputeSharedSecret 计算共享密钥
func (d *DHKeyExchange) ComputeSharedSecret(peerPublicKey []byte) ([]byte, error) {
	if len(peerPublicKey) != 64 {
		return nil, errors.New("invalid public key length")
	}

	// 解析对方的公钥
	peerX := new(big.Int).SetBytes(peerPublicKey[:32])
	peerY := new(big.Int).SetBytes(peerPublicKey[32:])

	// 验证公钥是否在曲线上
	if !d.curve.IsOnCurve(peerX, peerY) {
		return nil, errors.New("invalid public key: not on curve")
	}

	// 计算共享点
	x, y := d.curve.ScalarMult(peerX, peerY, d.privateKey)
	if x == nil || y == nil {
		return nil, errors.New("failed to compute shared secret")
	}

	// 使用X坐标派生密钥
	hash := sha256.Sum256(x.Bytes())
	return hash[:16], nil
}

// SendPublicKey 发送公钥到连接
func SendPublicKey(conn io.Writer, publicKey []byte) error {
	// ECDH公钥固定为64字节（32字节X + 32字节Y）
	if len(publicKey) != 64 {
		return errors.New("invalid ECDH public key length")
	}

	// 直接发送公钥数据
	if _, err := conn.Write(publicKey); err != nil {
		return err
	}

	return nil
}

// ReceivePublicKey 从连接接收公钥
func ReceivePublicKey(conn io.Reader) ([]byte, error) {
	// ECDH公钥固定为64字节
	publicKey := make([]byte, 64)
	if _, err := io.ReadFull(conn, publicKey); err != nil {
		return nil, err
	}

	return publicKey, nil
}

// PerformKeyExchange 执行完整的密钥交换流程
func PerformKeyExchange(conn io.ReadWriter) ([]byte, error) {
	// 步骤1：创建DH密钥交换实例
	dhExchange, err := NewDHKeyExchange()
	if err != nil {
		return nil, err
	}

	// 步骤2：发送自己的公钥
	if err := SendPublicKey(conn, dhExchange.GetPublicKey()); err != nil {
		return nil, err
	}

	// 步骤3：接收对方的公钥
	peerPublicKey, err := ReceivePublicKey(conn)
	if err != nil {
		return nil, err
	}

	// 步骤4：计算共享密钥
	sharedSecret, err := dhExchange.ComputeSharedSecret(peerPublicKey)
	if err != nil {
		return nil, err
	}

	return sharedSecret, nil
}
