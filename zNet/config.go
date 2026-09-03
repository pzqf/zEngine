package zNet

import "time"

// TcpConfig TCP服务器配置
// 定义TCP服务器运行所需的各项参数
type TcpConfig struct {
	ListenAddress        string        `toml:"listen_address" json:"listen_address"`
	MaxClientCount       int           `toml:"max_client_count" json:"max_client_count"`
	ChanSize             int           `toml:"chan_size" json:"chan_size"`
	HeartbeatDuration    int           `toml:"heartbeat_duration" json:"heartbeat_duration"`
	MaxPacketDataSize    int32         `toml:"max_packet_data_size" json:"max_packet_data_size"` // Deprecated: 使用 MaxWirePacketSize；保留旧配置键兼容
	MaxWirePacketSize    int32         `toml:"max_wire_packet_size" json:"max_wire_packet_size"`
	MaxDecodedPacketSize int32         `toml:"max_decoded_packet_size" json:"max_decoded_packet_size"`
	UseWorkerPool        bool          `toml:"use_worker_pool" json:"use_worker_pool"`
	WorkerPoolSize       int           `toml:"worker_pool_size" json:"worker_pool_size"`
	WorkerQueueSize      int           `toml:"worker_queue_size" json:"worker_queue_size"`
	DisableEncryption    bool          `toml:"disable_encryption" json:"disable_encryption"`
	DisableCompression   bool          `toml:"disable_compression" json:"disable_compression"`
	TcpNoDelay           bool          `toml:"tcp_no_delay" json:"tcp_no_delay"`
	WriteBufferSize      int           `toml:"write_buffer_size" json:"write_buffer_size"`
	ReadBufferSize       int           `toml:"read_buffer_size" json:"read_buffer_size"`
	EnableKeyRotation    bool          `toml:"enable_key_rotation" json:"enable_key_rotation"`
	KeyRotationInterval  time.Duration `toml:"key_rotation_interval" json:"key_rotation_interval"`
	MaxHistoryKeys       int           `toml:"max_history_keys" json:"max_history_keys"`
	EnableSequenceCheck  bool          `toml:"enable_sequence_check" json:"enable_sequence_check"`
	SequenceWindowSize   uint64        `toml:"sequence_window_size" json:"sequence_window_size"`
	TimestampTolerance   int64         `toml:"timestamp_tolerance" json:"timestamp_tolerance"`
	ByteOrder            WireByteOrder `toml:"byte_order" json:"byte_order"`

	ProtocolVersion ProtocolVersionPolicy `toml:"protocol_version" json:"protocol_version"`
}

// UdpConfig UDP服务器配置
// 定义UDP服务器运行所需的各项参数
type UdpConfig struct {
	ListenAddress        string        `toml:"listen_address" json:"listen_address"`             // 监听地址，默认 ":9016"
	MaxClientCount       int           `toml:"max_client_count" json:"max_client_count"`         // 最大客户端连接数，默认 10000
	ChanSize             int           `toml:"chan_size" json:"chan_size"`                       // Session收发通道大小，默认 512
	HeartbeatDuration    int           `toml:"heartbeat_duration" json:"heartbeat_duration"`     // 心跳间隔时间（秒），默认 30
	MaxPacketDataSize    int32         `toml:"max_packet_data_size" json:"max_packet_data_size"` // Deprecated: 使用 MaxWirePacketSize
	MaxWirePacketSize    int32         `toml:"max_wire_packet_size" json:"max_wire_packet_size"`
	MaxDecodedPacketSize int32         `toml:"max_decoded_packet_size" json:"max_decoded_packet_size"`
	ByteOrder            WireByteOrder `toml:"byte_order" json:"byte_order"`
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
	ListenAddress        string        `toml:"listen_address" json:"listen_address"`             // 监听地址，默认 ":9016"
	MaxClientCount       int           `toml:"max_client_count" json:"max_client_count"`         // 最大客户端连接数，默认 10000
	ChanSize             int           `toml:"chan_size" json:"chan_size"`                       // Session收发通道大小，默认 512
	HeartbeatDuration    int           `toml:"heartbeat_duration" json:"heartbeat_duration"`     // 心跳间隔时间（秒），默认 30
	MaxPacketDataSize    int32         `toml:"max_packet_data_size" json:"max_packet_data_size"` // Deprecated: 使用 MaxWirePacketSize
	MaxWirePacketSize    int32         `toml:"max_wire_packet_size" json:"max_wire_packet_size"`
	MaxDecodedPacketSize int32         `toml:"max_decoded_packet_size" json:"max_decoded_packet_size"`
	ByteOrder            WireByteOrder `toml:"byte_order" json:"byte_order"`
}

// TcpClientConfig TCP客户端配置
// 定义TCP客户端运行所需的各项参数
type TcpClientConfig struct {
	ServerAddr           string            `toml:"server_addr" json:"server_addr"`                   // 服务器地址
	ServerPort           int               `toml:"server_port" json:"server_port"`                   // 服务器端口
	ChanSize             int               `toml:"chan_size" json:"chan_size"`                       // Session收发通道大小，默认 1024
	HeartbeatDuration    int               `toml:"heartbeat_duration" json:"heartbeat_duration"`     // 心跳间隔时间（秒），默认 30
	MaxPacketDataSize    int32             `toml:"max_packet_data_size" json:"max_packet_data_size"` // Deprecated: 使用 MaxWirePacketSize
	MaxWirePacketSize    int32             `toml:"max_wire_packet_size" json:"max_wire_packet_size"`
	MaxDecodedPacketSize int32             `toml:"max_decoded_packet_size" json:"max_decoded_packet_size"`
	AutoReconnect        bool              `toml:"auto_reconnect" json:"auto_reconnect"`           // 是否自动重连，默认 false
	ReconnectDelay       int               `toml:"reconnect_delay" json:"reconnect_delay"`         // 重连延迟（秒），默认 5
	MaxReconnectTimes    int               `toml:"max_reconnect_times" json:"max_reconnect_times"` // 最大重连次数，默认 0（无限重连）
	DisableEncryption    bool              `toml:"disable_encryption" json:"disable_encryption"`   // 是否禁用加密，默认 false
	Compression          CompressionConfig `toml:"compression" json:"compression"`                 // 压缩配置
	ByteOrder            WireByteOrder     `toml:"byte_order" json:"byte_order"`                   // 包头字节序，默认快照全局兼容值

	ProtocolVersion            ProtocolVersionPolicy `toml:"protocol_version" json:"protocol_version"`                         // 显式启用；零值保持 Version=0
	ProtocolNegotiationTimeout time.Duration         `toml:"protocol_negotiation_timeout" json:"protocol_negotiation_timeout"` // 仅启用版本协商时生效
}

// DDoSConfig DDoS保护配置
// 定义DDoS防护的各项阈值参数
//
// ⚠ 统计维度不一样，看清楚再调：
//   - MaxConnPerIP / BanDuration 是**按 IP** 的（建连时还没有任何会话身份，只能按 IP）。
//     超限会拉黑该 IP —— 这是唯一会拉黑 IP 的路径。
//   - MaxPacketsPerIP / MaxBytesPerIP 名字带 PerIP 是历史遗留，**实际按连接**计
//     （见 DDoSProtection.AllowPacketFrom / AllowTrafficFrom，主体为 "ip#sid"）。
//     超限只断那一条连接、**不拉黑 IP**：NAT/CGNAT 下同 IP 后面可能是成千上万无关玩家，
//     按 IP 聚合会让一个人的突发把整段 IP 的人一起封掉。配置键名未改以免破坏既有 ini。
type DDoSConfig struct {
	MaxConnPerIP      int      `toml:"max_conn_per_ip" json:"max_conn_per_ip"`
	ConnTimeWindow    int      `toml:"conn_time_window" json:"conn_time_window"`
	MaxPacketsPerIP   int      `toml:"max_packets_per_ip" json:"max_packets_per_ip"` // 实为每连接
	PacketTimeWindow  int      `toml:"packet_time_window" json:"packet_time_window"`
	MaxBytesPerIP     int64    `toml:"max_bytes_per_ip" json:"max_bytes_per_ip"` // 实为每连接
	TrafficTimeWindow int      `toml:"traffic_time_window" json:"traffic_time_window"`
	BanDuration       int      `toml:"ban_duration" json:"ban_duration"`
	WhitelistIPs      []string `toml:"whitelist_ips" json:"whitelist_ips"`
}

// DefaultDDoSConfig 默认DDoS保护配置
// 返回预配置的DDoS保护参数
//
// 返回:
//   - *DDoSConfig: 默认DDoS配置实例
func DefaultDDoSConfig() *DDoSConfig {
	return &DDoSConfig{
		MaxConnPerIP:      10000,
		ConnTimeWindow:    600,
		MaxPacketsPerIP:   10000,
		PacketTimeWindow:  1,
		MaxBytesPerIP:     1024 * 1024 * 1024,
		TrafficTimeWindow: 3600,
		BanDuration:       60,
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
