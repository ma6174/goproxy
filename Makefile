All:
	go build proxy.go

arm:
	gox -osarch="linux/arm"

clean:
	rm ./goproxy_linux_arm
	rm ./proxy
