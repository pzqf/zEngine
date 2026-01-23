package zNet

type TcpConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             //default ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`         //default 10000
	ChanSize          int    `toml:"chan_size" json:"chan_size"`                       //session receive and send chanel size, default 1024
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"`     //default 30s
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` //default 1024*1024
}

type UdpConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             //default ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`         //default 10000
	ChanSize          int    `toml:"chan_size" json:"chan_size"`                       //session receive and send chanel size, default 512
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"`     //default 30s
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` //default 1024*1024
}

type HttpConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             //default ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`         //default 10000
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` //default 1024*1024
}

type WebSocketConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             //default ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`         //default 10000
	ChanSize          int    `toml:"chan_size" json:"chan_size"`                       //session receive and send chanel size, default 512
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"`     //default 30s
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` //default 1024*1024
}

// DDoSConfig DDoS保护配置
type DDoSConfig struct {
	MaxConnPerIP      int   `toml:"max_conn_per_ip" json:"max_conn_per_ip"`         //每个IP最大连接数，默认10
	ConnTimeWindow    int   `toml:"conn_time_window" json:"conn_time_window"`       //连接时间窗口（秒），默认60
	MaxPacketsPerIP   int   `toml:"max_packets_per_ip" json:"max_packets_per_ip"`   //每个IP最大数据包数，默认100
	PacketTimeWindow  int   `toml:"packet_time_window" json:"packet_time_window"`   //数据包时间窗口（秒），默认1
	MaxBytesPerIP     int64 `toml:"max_bytes_per_ip" json:"max_bytes_per_ip"`       //每个IP最大流量（字节），默认10MB
	TrafficTimeWindow int   `toml:"traffic_time_window" json:"traffic_time_window"` //流量时间窗口（秒），默认3600
	BanDuration       int   `toml:"ban_duration" json:"ban_duration"`               //黑名单持续时间（秒），默认86400
}

// DefaultDDoSConfig 默认DDoS保护配置
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
