module github.com/pzqf/zEngine

go 1.18

require github.com/pzqf/zUtil v0.0.1

require (
	github.com/dennwc/graphml v0.0.0-20180609132439-6d40272e8e4b
	github.com/gorilla/websocket v1.5.0
	github.com/panjf2000/ants v1.3.0
	github.com/pkg/profile v1.6.0
	go.etcd.io/etcd/api/v3 v3.5.4
	go.etcd.io/etcd/client/v3 v3.5.4
	go.uber.org/zap v1.21.0
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
)

require (
	github.com/BurntSushi/toml v1.1.0 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.4 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/sys v0.0.0-20210603081109-ebe580a85c40 // indirect
	golang.org/x/text v0.3.5 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/grpc v1.38.0 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
)

replace github.com/pzqf/zUtil => ../zUtil
