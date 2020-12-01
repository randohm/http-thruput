NAME=http-thruput

${NAME}: main.go
	go build -o ${NAME}

all: ${NAME} linux arm64

linux:
	GOOS=linux go build -o ${NAME}.linux

arm64:
	GOOS=linux GOARCH=arm64 go build -o ${NAME}.arm64

copy:
	for I in nova nuc{1..3} nyquist ; do scp ${NAME}.linux $$I:${NAME} & done
	for I in rpi4-{1..3} ; do scp ${NAME}.arm64 $$I:${NAME} & done

clean:
	rm -f ${NAME} ${NAME}.linux ${NAME}.arm64
