all: install

fmt:
	go fmt ./...

install: fmt
	go install ./...

test: install
	go test -short ./...

test-long: install
	go test -v -race -tags=debug ./...

# run twice to ensure references are updated properly
whitepaper:
	@pdflatex whitepaper.tex > /dev/null
	pdflatex whitepaper.tex

dependencies:
	go install -race std
	go get -u code.google.com/p/gcfg
	go get -u github.com/mitchellh/go-homedir
	go get -u github.com/spf13/cobra

clean:
	rm -rf host release whitepaper.aux whitepaper.log whitepaper.pdf

# Cross Compile - makes binaries for windows, linux, and mac, 32 and 64 bit.
xc:
	go get -u github.com/laher/goxc
	goxc -arch="386 amd64" -bc="linux,!arm windows darwin" -d=release -pv=0.1.0 -br=developer -pr=beta -include=style/,example-config,LICENSE*,README* -tasks-=deb,deb-dev,deb-source -build-tags=release

.PHONY: all fmt install test test-long test-long-race whitepaper dependencies distribution clean xc
