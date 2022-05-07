.PHONY: build
build:
	mkdir -p bin/ && go build -o ./bin/server.exe main_server.go
	mkdir -p bin/ && go build -o ./bin/client.exe main_client.go

.PHONY: linux
linux:
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/server main_server.go
	mkdir -p bin/ &&env GOOS=linux GOARCH=amd64 go build  -o ./bin/client main_client.go

