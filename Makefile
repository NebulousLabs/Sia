all: install

fmt:
	go fmt ./...

install: fmt
	go install ./...

test: install
	go test -short ./...

test-long: install
	go test -v -race ./...

whitepaper:
	pdflatex whitepaper.tex
