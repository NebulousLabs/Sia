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
	go get -u golang.org/x/crypto/twofish
	# Module + Daemon Dependencies
	go get -u github.com/NebulousLabs/entropy-mnemonics
	go get -u github.com/NebulousLabs/go-upnp
	go get -u github.com/inconshreveable/go-update
	go get -u github.com/inconshreveable/muxado
	go get -u github.com/kardianos/osext
	go get -u github.com/klauspost/reedsolomon
	go get -u github.com/julienschmidt/httprouter
	# Frontend Dependencies
	go get -u github.com/bgentry/speakeasy
	go get -u github.com/spf13/cobra
	# Developer Dependencies
	go install -race std
	go get -u github.com/golang/lint/golint
	go get -u github.com/laher/goxc
	go get -u golang.org/x/tools/cmd/cover

# pkgs changes which packages the makefile calls operate on. run changes which
# tests are run during testing.
run = Test
pkgs = ./api ./build ./compatibility ./crypto ./encoding ./modules ./modules/consensus \
       ./modules/explorer ./modules/gateway ./modules/host ./modules/renter/hostdb \
       ./modules/miner ./modules/renter ./modules/transactionpool ./modules/wallet \
       ./persist ./siac ./siad ./sync ./types

# fmt calls go fmt on all packages.
fmt:
	go fmt $(pkgs)

# vet calls go vet on all packages.
vet:
	go vet $(pkgs)

# will always run on some packages for a while.
lintpkgs = ./modules/host ./modules
lint:
	@for package in $(lintpkgs); do                           \
		golint -min_confidence=1.0 $$package                  \
		&& test -z $$(golint -min_confidence=1.0 $$package) ; \
	done

# REBUILD touches all of the build-dependent source files, forcing them to be
# rebuilt. This is necessary because the go tool is not smart enough to trigger
# a rebuild when build tags have been changed.
REBUILD:
	@touch build/*.go

# install builds and installs developer binaries.
install: REBUILD
	go install -race -tags='dev debug profile' ./...

# release builds and installs release binaries.
release: REBUILD
	go install -a -race -tags='debug profile' ./...
release-std: REBUILD
	go install -a ./...

# xc builds and packages release binaries for all systems by using goxc.
# Cross Compile - makes binaries for windows, linux, and mac, 32 and 64 bit.
xc: dependencies test test-long REBUILD
	goxc -arch="386 amd64 arm" -bc="darwin linux windows" -d=release \
	     -pv=v0.5.1 -br=beta -include=LICENSE,README.md,doc/API.md   \
	     -tasks-=deb,deb-dev,deb-source,go-test -n=Sia

# clean removes all directories that get automatically created during
# development.
clean:
	rm -rf release doc/whitepaper.aux doc/whitepaper.log doc/whitepaper.pdf

test: REBUILD
	go test -short -tags='debug testing' -timeout=3s $(pkgs) -run=$(run)
test-v: REBUILD
	go test -race -v -short -tags='debug testing' -timeout=15s $(pkgs) -run=$(run)
test-long: clean fmt vet lint REBUILD
	go test -v -race -tags='testing debug' -timeout=300s $(pkgs) -run=$(run)
bench: clean fmt REBUILD
	go test -tags='testing' -timeout=300s -run=XXX -bench=. $(pkgs)
cover: clean REBUILD
	@mkdir -p cover/modules
	@mkdir -p cover/modules/renter
	@for package in $(pkgs); do                                                                                     \
		go test -tags='testing debug' -timeout=360s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done
cover-integration: clean REBUILD
	@mkdir -p cover/modules
	@mkdir -p cover/modules/renter
	@for package in $(pkgs); do                                                                                     \
		go test -run=TestIntegration -tags='testing debug' -timeout=360s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done
cover-unit: clean REBUILD
	@mkdir -p cover/modules
	@mkdir -p cover/modules/renter
	@for package in $(pkgs); do                                                                                     \
		go test -run=TestUnit -tags='testing debug' -timeout=360s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done

# whitepaper builds the whitepaper from whitepaper.tex. pdflatex has to be
# called twice because references will not update correctly the first time.
whitepaper:
	@pdflatex -output-directory=doc whitepaper.tex > /dev/null
	pdflatex -output-directory=doc whitepaper.tex

.PHONY: all dependencies fmt REBUILD install release release-std xc clean test test-v test-long cover cover-integration cover-unit whitepaper
