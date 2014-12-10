all: install

fmt:
	go fmt ./...

install: fmt
	go install ./...

test: install
	go test -short ./...

test-long: install
	go test -v ./...

test-long-race: install
	go test -v -race ./...

# run twice to ensure references are updated properly
whitepaper:
	@pdflatex whitepaper.tex > /dev/null
	pdflatex whitepaper.tex

race-libs:
	go install -race std

dependencies: race-libs
	go get -u code.google.com/p/gcfg
	go get -u github.com/mitchellh/go-homedir
	go get -u github.com/spf13/cobra

distribution:
	go install ./...
	cp $(GOPATH)/bin/siad config/siad
	tar -cJvf sia-bundle.xz config/*
	rm -f config/siad

.PHONY: all fmt install test test-long test-long-race whitepaper race-libs dependencies distribution
