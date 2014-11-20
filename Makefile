all: install

fmt:
	go fmt ./...

install: fmt
	go install ./...

test: install
	go test -short ./...

test-long: install
	got test -v -race ./...

whitepaper:
	pdflatex whitepaper.tex
	pdflatex whitepaper.tex
	pdflatex whitepaper.tex # pdfatex is dumb, therefore we run it 3 times - fixes errors that could occur from only running it once or twice.
