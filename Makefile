all: http-bandwidth-test

http-bandwidth-test: main.go
	go build

linux:
	GOOS=linux go build -o http-bandwidth-test.linux

arm64:
	GOOS=linux GOARCH=arm64 go build -o http-bandwidth-test.arm64
