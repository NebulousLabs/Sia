# all will build and install developer binaries, which have debugging enabled
# and much faster mining and block constants.
all: install

# dependencies installs all of the dependencies that are required for building
# Sia.
dependencies:
	# Consensus Dependencies
	go get -u github.com/NebulousLabs/demotemutex
	go get -u github.com/NebulousLabs/ed25519
	go get -u github.com/NebulousLabs/merkletree
	go get -u github.com/NebulousLabs/bolt
	go get -u github.com/dchest/blake2b
	# Module + Daemon Dependencies
	go get -u github.com/NebulousLabs/entropy-mnemonics
	go get -u github.com/NebulousLabs/go-upnp
	go get -u github.com/NebulousLabs/muxado
	go get -u github.com/klauspost/reedsolomon
	go get -u github.com/julienschmidt/httprouter
	go get -u github.com/inconshreveable/go-update
	go get -u github.com/kardianos/osext
	# Frontend Dependencies
	go get -u github.com/bgentry/speakeasy
	go get -u github.com/spf13/cobra
	# Developer Dependencies
	go install -race std
	go get -u github.com/golang/lint/golint
	go get -u github.com/laher/goxc
	go get -u github.com/mattn/goveralls


# pkgs changes which packages the makefile calls operate on. run changes which
# tests are run during testing.
run = Test
pkgs = ./api ./build ./compatibility ./crypto ./encoding ./modules ./modules/consensus \
       ./modules/explorer ./modules/gateway ./modules/host ./modules/host/storagemanager \
       ./modules/renter ./modules/renter/contractor ./modules/renter/hostdb ./modules/renter/proto \
       ./modules/miner ./modules/wallet ./modules/transactionpool ./persist ./siac ./siad ./sync ./types

# fmt calls go fmt on all packages.
fmt:
	gofmt -s -l -w $(pkgs)

# vet calls go vet on all packages.
# NOTE: go vet requires packages to be built in order to obtain type info.
vet: release-std
	go vet $(pkgs)

# will always run on some packages for a while.
lintpkgs = ./modules/host ./modules ./modules/renter/hostdb ./modules/renter/contractor ./persist
lint:
	@for package in $(lintpkgs); do                           \
		golint -min_confidence=1.0 $$package                  \
		&& test -z $$(golint -min_confidence=1.0 $$package) ; \
	done

# install builds and installs developer binaries.
install:
	go install -race -tags='dev debug profile' $(pkgs)

# release builds and installs release binaries.
release:
	go install -tags='debug profile' $(pkgs)
release-race:
	go install -race -tags='debug profile' $(pkgs)
release-std:
	go install $(pkgs)

# xc builds and packages release binaries for all systems by using goxc.
# Cross Compile - makes binaries for windows, linux, and mac, 64 bit only.
xc: dependencies test test-long
	goxc -arch="amd64" -bc="darwin linux windows" -d=release \
	     -pv=v1.0.0 -include=LICENSE,README.md,doc/API.md \
	     -tasks-=archive,rmbin,deb,deb-dev,deb-source,go-test -n=Sia

# clean removes all directories that get automatically created during
# development.
clean:
	rm -rf release doc/whitepaper.aux doc/whitepaper.log doc/whitepaper.pdf

test:
	go test -short -tags='debug testing' -timeout=5s $(pkgs) -run=$(run)
test-v:
	go test -race -v -short -tags='debug testing' -timeout=15s $(pkgs) -run=$(run)
test-long: clean fmt vet lint
	go test -v -race -tags='testing debug' -timeout=500s $(pkgs) -run=$(run)
bench: clean fmt
	go test -tags='testing' -timeout=500s -run=XXX -bench=. $(pkgs)
cover: clean
	@mkdir -p cover/modules
	@mkdir -p cover/modules/renter
	@mkdir -p cover/modules/host
	@for package in $(pkgs); do                                                                                     \
		go test -tags='testing debug' -timeout=500s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done
cover-integration: clean
	@mkdir -p cover/modules
	@mkdir -p cover/modules/renter
	@mkdir -p cover/modules/host
	@for package in $(pkgs); do                                                                                     \
		go test -run=TestIntegration -tags='testing debug' -timeout=500s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done
cover-unit: clean
	@mkdir -p cover/modules
	@mkdir -p cover/modules/renter
	@mkdir -p cover/modules/host
	@for package in $(pkgs); do                                                                                     \
		go test -run=TestUnit -tags='testing debug' -timeout=500s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done

# whitepaper builds the whitepaper from whitepaper.tex. pdflatex has to be
# called twice because references will not update correctly the first time.
whitepaper:
	@pdflatex -output-directory=doc whitepaper.tex > /dev/null
	pdflatex -output-directory=doc whitepaper.tex

.PHONY: all dependencies fmt install release release-std xc clean test test-v test-long cover cover-integration cover-unit whitepaper
