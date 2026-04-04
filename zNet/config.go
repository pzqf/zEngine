package zNet

import "time"

// TcpConfig TCP服务器配置
// 定义TCP服务器运行所需的各项参数
type TcpConfig struct {
	ListenAddress       string        `toml:"listen_address" json:"listen_address"`               // 监听地址，默认 ":9016"
	MaxClientCount      int           `toml:"max_client_count" json:"max_client_count"`           // 最大客户端连接数，默认 10000
	ChanSize            int           `toml:"chan_size" json:"chan_size"`                         // Session收发通道大小，默认 1024
	HeartbeatDuration   int           `toml:"heartbeat_duration" json:"heartbeat_duration"`       // 心跳间隔时间（秒），默认 30
	MaxPacketDataSize   int32         `toml:"max_packet_data_size" json:"max_packet_data_size"`   // 最大数据包大小，默认 1024*1024
	UseWorkerPool       bool          `toml:"use_worker_pool" json:"use_worker_pool"`             // 是否使用工作池模式，默认 false
	WorkerPoolSize      int           `toml:"worker_pool_size" json:"worker_pool_size"`           // 工作池大小，默认 100
	WorkerQueueSize     int           `toml:"worker_queue_size" json:"worker_queue_size"`         // 工作池队列大小，默认 10000
	DisableEncryption   bool          `toml:"disable_encryption" json:"disable_encryption"`       // 是否禁用加密，默认 false
	EnableKeyRotation   bool          `toml:"enable_key_rotation" json:"enable_key_rotation"`     // 是否启用密钥轮换，默认 false
	KeyRotationInterval time.Duration `toml:"key_rotation_interval" json:"key_rotation_interval"` // 密钥轮换间隔，默认 30分钟
	MaxHistoryKeys      int           `toml:"max_history_keys" json:"max_history_keys"`           // 保留历史密钥数量，默认 3
	EnableSequenceCheck bool          `toml:"enable_sequence_check" json:"enable_sequence_check"` // 是否启用序列号检查，默认 false
	SequenceWindowSize  uint64        `toml:"sequence_window_size" json:"sequence_window_size"`   // 序列号窗口大小，默认 1000
	TimestampTolerance  int64         `toml:"timestamp_tolerance" json:"timestamp_tolerance"`     // 时间戳容忍度（秒），默认 30
}

// UdpConfig UDP服务器配置
// 定义UDP服务器运行所需的各项参数
type UdpConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             // 监听地址，默认 ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`         // 最大客户端连接数，默认 10000
	ChanSize          int    `toml:"chan_size" json:"chan_size"`                       // Session收发通道大小，默认 512
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"`     // 心跳间隔时间（秒），默认 30
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` // 最大数据包大小，默认 1024*1024
}

// HttpConfig HTTP服务器配置
// 定义HTTP服务器运行所需的各项参数
type HttpConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             // 监听地址，默认 ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`         // 最大客户端连接数，默认 10000
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` // 最大数据包大小，默认 1024*1024
}

// WebSocketConfig WebSocket服务器配置
// 定义WebSocket服务器运行所需的各项参数
type WebSocketConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             // 监听地址，默认 ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`         // 最大客户端连接数，默认 10000
	ChanSize          int    `toml:"chan_size" json:"chan_size"`                       // Session收发通道大小，默认 512
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"`     // 心跳间隔时间（秒），默认 30
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` // 最大数据包大小，默认 1024*1024
}

// TcpClientConfig TCP客户端配置
// 定义TCP客户端运行所需的各项参数
type TcpClientConfig struct {
	ServerAddr        string            `toml:"server_addr" json:"server_addr"`                   // 服务器地址
	ServerPort        int               `toml:"server_port" json:"server_port"`                   // 服务器端口
	ChanSize          int               `toml:"chan_size" json:"chan_size"`                       // Session收发通道大小，默认 1024
	HeartbeatDuration int               `toml:"heartbeat_duration" json:"heartbeat_duration"`     // 心跳间隔时间（秒），默认 30
	MaxPacketDataSize int32             `toml:"max_packet_data_size" json:"max_packet_data_size"` // 最大数据包大小，默认 1024*1024
	AutoReconnect     bool              `toml:"auto_reconnect" json:"auto_reconnect"`             // 是否自动重连，默认 false
	ReconnectDelay    int               `toml:"reconnect_delay" json:"reconnect_delay"`           // 重连延迟（秒），默认 5
	MaxReconnectTimes int               `toml:"max_reconnect_times" json:"max_reconnect_times"`   // 最大重连次数，默认 0（无限重连）
	DisableEncryption bool              `toml:"disable_encryption" json:"disable_encryption"`     // 是否禁用加密，默认 false
	Compression       CompressionConfig `toml:"compression" json:"compression"`                   // 压缩配置
}

// DDoSConfig DDoS保护配置
// 定义DDoS防护的各项阈值参数
type DDoSConfig struct {
	MaxConnPerIP      int   `toml:"max_conn_per_ip" json:"max_conn_per_ip"`         // 每个IP最大连接数，默认 10
	ConnTimeWindow    int   `toml:"conn_time_window" json:"conn_time_window"`       // 连接时间窗口（秒），默认 60
	MaxPacketsPerIP   int   `toml:"max_packets_per_ip" json:"max_packets_per_ip"`   // 每个IP最大数据包数，默认 100
	PacketTimeWindow  int   `toml:"packet_time_window" json:"packet_time_window"`   // 数据包时间窗口（秒），默认 1
	MaxBytesPerIP     int64 `toml:"max_bytes_per_ip" json:"max_bytes_per_ip"`       // 每个IP最大流量（字节），默认 10MB
	TrafficTimeWindow int   `toml:"traffic_time_window" json:"traffic_time_window"` // 流量时间窗口（秒），默认 3600
	BanDuration       int   `toml:"ban_duration" json:"ban_duration"`               // 黑名单持续时间（秒），默认 86400
}

// DefaultDDoSConfig 默认DDoS保护配置
// 返回预配置的DDoS保护参数
//
// 返回:
//   - *DDoSConfig: 默认DDoS配置实例
func DefaultDDoSConfig() *DDoSConfig {
	return &DDoSConfig{
		MaxConnPerIP:      10,
		ConnTimeWindow:    60,
		MaxPacketsPerIP:   100,
		PacketTimeWindow:  1,
		MaxBytesPerIP:     10 * 1024 * 1024,
		TrafficTimeWindow: 3600,
		BanDuration:       24 * 3600,
	}
}

// CompressionConfig 压缩配置
// 定义压缩功能的各项参数
type CompressionConfig struct {
	Enabled              bool `toml:"enabled" json:"enabled"`                             // 是否启用压缩，默认 true
	CompressionThreshold int  `toml:"compression_threshold" json:"compression_threshold"` // 压缩阈值，默认 1024
	MaxCompressSize      int  `toml:"max_compress_size" json:"max_compress_size"`         // 最大压缩大小，默认 1024*1024
}

// DefaultCompressionConfig 默认压缩配置
// 返回预配置的压缩参数
//
// 返回:
//   - *CompressionConfig: 默认压缩配置实例
func DefaultCompressionConfig() *CompressionConfig {
	return &CompressionConfig{
		Enabled:              true,
		CompressionThreshold: 1024,
		MaxCompressSize:      1024 * 1024,
	}
}
