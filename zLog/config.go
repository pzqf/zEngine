package zLog

type LogConfig struct {
	Level         int    `json:"level"`           //default LevelNil
	LogDir        string `json:"log_dir"`         // if empty, don't output file
	LogFilePrefix string `json:"log_file_prefix"` // if empty, application name
	Console       bool   `json:"console"`         //default true
	CallerDepth   int    `json:"caller_depth"`    //default 2
	MsgChanLen    int    `json:"msg_chan_len"`    //default 512
	Daily         bool   `json:"daily"`           //default false
	MaxLine       int64  `json:"max_line"`        //default 10000000
	MaxSize       int64  `json:"max_size"`        //default 2G
	OutputJson    bool   `json:"output_json"`     //default false
}
