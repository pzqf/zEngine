package zNet

type Config struct {
	Tcp       *TcpConfig       `toml:"tcp" json:"tcp"`
	Udp       *UdpConfig       `toml:"udp" json:"udp"`
	Http      *HttpConfig      `toml:"http" json:"http"`
	WebSocket *WebSocketConfig `toml:"web_socket" json:"web_socket"`

	MaxPacketDataSize int `toml:"max_packet_data_size" json:"max_packet_data_size"` //default 1024*1024
}

type TcpConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`         //default ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`     //default 10000
	ChanSize          int    `toml:"chan_size" json:"chan_size"`                   //session receive and send chanel size, default 512
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"` //default 30s
}

type UdpConfig struct {
	ListenAddress     string `toml:"listen_address" json:"listen_address"`         //default ":9016"
	MaxClientCount    int    `toml:"max_client_count" json:"max_client_count"`     //default 10000
	ChanSize          int    `toml:"chan_size" json:"chan_size"`                   //session receive and send chanel size, default 512
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"` //default 30s
}

type HttpConfig struct {
}

type WebSocketConfig struct {
	ListenAddress string `toml:"listen_address" json:"listen_address"` //default ":9016"
	ChanSize      int    `toml:"chan_size" json:"chan_size"`           //session receive and send chanel size, default 512
}
