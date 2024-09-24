la64:
	CC=loongarch64-unknown-linux-gnu-gcc CXX=loongarch64-unknown-linux-gnu-g++ GOOS=linux GOARCH=loong64 go build
	loongarch64-unknown-linux-gnu-strip http-server

default:
	go build
	strip http-server