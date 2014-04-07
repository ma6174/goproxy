All:
	go build proxy.go

arm:
	gox -osarch="linux/arm" -verbose -output="goproxy_linux_arm"

clean:
	rm ./goproxy_linux_arm
	rm ./proxy
