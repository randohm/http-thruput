NAME=http-thruput

${NAME}: main.go
	go build -o ${NAME}

all: ${NAME} linux arm64

linux:
	GOOS=linux go build -o ${NAME}.linux

arm64:
	GOOS=linux GOARCH=arm64 go build -o ${NAME}.arm64

clean:
	rm -f ${NAME} ${NAME}.linux ${NAME}.arm64
