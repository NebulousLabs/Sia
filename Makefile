all: install

fmt:
	go fmt ./...

install: fmt
	go install ./...

whitepaper:
	pdflatex whitepaper.tex
