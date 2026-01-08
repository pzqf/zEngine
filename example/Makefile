.PHONY: tcp_win
tcp_win:
	mkdir -p bin/ && go build -o ./bin/engine_test_tcp_server.exe main_tcp_server.go
	mkdir -p bin/ && go build -o ./bin/engine_test_tcp_client.exe main_tcp_client.go

.PHONY: tcp_linux
tcp_linux:
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_tcp_server main_tcp_server.go
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_tcp_client main_tcp_client.go

.PHONY: script_win
script_win:
	mkdir -p bin/ && go build -o ./bin/engine_test_script.exe main_script.go

.PHONY: script_linux
script_linux:
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_script main_script.go

