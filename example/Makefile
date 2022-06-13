.PHONY: tcp_win
tcp_win:
	mkdir -p bin/ && go build -o ./bin/engine_test_tcp_server.exe main_server.go
	mkdir -p bin/ && go build -o ./bin/engine_test_tcp_client.exe main_client.go

.PHONY: tcp_linux
tcp_linux:
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_tcp_server main_server.go
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_tcp_client main_client.go


.PHONY: udp_win
udp_win:
	mkdir -p bin/ && go build -o ./bin/engine_test_udp_server.exe main_udp_server.go
	mkdir -p bin/ && go build -o ./bin/engine_test_udp_client.exe main_udp_client.go

.PHONY: udp_linux
udp_linux:
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_udp_server main_udp_server.go
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_udp_client main_udp_client.go

