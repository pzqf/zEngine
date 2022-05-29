.PHONY: win
win:
	mkdir -p bin/ && go build -o ./bin/engine_test_server.exe main_server.go
	mkdir -p bin/ && go build -o ./bin/engine_test_client.exe main_client.go

.PHONY: linux
linux:
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_server main_server.go
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/engine_test_client main_client.go

