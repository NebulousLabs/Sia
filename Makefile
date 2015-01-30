all: install

fmt:
	go fmt ./...

install: fmt
	go install -a -tags=dev ./...

clean:
	rm -rf hostdir release whitepaper.aux whitepaper.log whitepaper.pdf         \
		sia.wallet sia/test.wallet sia/hostdir* sia/renterDownload

# Touching a file in the consensus folder forces the build tag files to be
# rebuilt. This can also be achieved with 'go test -a', however using the '-a'
# flag results in a multi-second compile time, which is undesirable. Leaving
# out both the touch and the '-a' means that sometimes the tests will be run
# using the developer constants, which is very, very slow.
test: clean fmt
	touch consensus/blocknode.go
	go test -short -tags=test ./...


test-long: clean fmt
	go test -a -v -race -short -tags=test ./...
	go test -a -v -race -tags=test ./...

# run twice to ensure references are updated properly
whitepaper:
	@pdflatex whitepaper.tex > /dev/null
	pdflatex whitepaper.tex

dependencies:
	go install -race std
	go get -u code.google.com/p/gcfg
	go get -u github.com/mitchellh/go-homedir
	go get -u github.com/spf13/cobra
	go get -u github.com/inconshreveable/go-update
	go get -u github.com/agl/ed25519
	go get -u golang.org/x/crypto/twofish
	go get -u github.com/stretchr/graceful
	go get -u github.com/laher/goxc

release: dependencies test-long
	go install -a ./...

# Cross Compile - makes binaries for windows, linux, and mac, 32 and 64 bit.
xc: dependencies test-long
	goxc -arch="amd64" -bc="linux windows darwin" -d=release -pv=0.2.0          \
		-br=release -pr=beta -include=example-config,LICENSE*,README*           \
		-tasks-=deb,deb-dev,deb-source,go-test
	# Need some command here to make sure that the release constants got used.

.PHONY: all fmt install clean test test-long whitepaper dependencies release xc
