package zLog

func ExampleSetLogger() {
	SetLogger(`{
			"level": 1,
			"log_dir": "./logs",
			"log_file_prefix": "app",
			"console": true,
			"caller_depth": 2,
			"msg_chan_len": 512,
			"daily": true,
			"max_line": 100000000,
			"max_size": 2147483648,
			"output_json": false
		}`,
	)
}

func ExampleNewLogger() {
	logger := NewLogger(`{
			"level": 1,
			"log_dir": "./logs",
			"log_file_prefix": "app",
			"console": true,
			"caller_depth": 2,
			"msg_chan_len": 512,
			"daily": true,
			"max_line": 100000000,
			"max_size": 2147483648,
			"output_json": false
		}`,
	)
	logger.Info("test")
}
