package zNet

type Config struct {
	MaxPacketDataSize int32  `toml:"max_packet_data_size" json:"max_packet_data_size"` //default 1024*1024
	ListenAddress     string `toml:"listen_address" json:"listen_address"`             //default ":9016"
	MaxClientCount    int32  `toml:"max_client_count" json:"max_client_count"`         //default 10000
	ChanSize          int32  `toml:"chan_size" json:"chan_size"`                       //session receive and send chanel size, default 2048
	HeartbeatDuration int    `toml:"heartbeat_duration" json:"heartbeat_duration"`     //default 30s
}

var GConfig *Config
