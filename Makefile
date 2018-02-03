# all will build and install developer binaries, which have debugging enabled
# and much faster mining and block constants.
all: release-std

# dependencies installs all of the dependencies that are required for building
# Sia.
dependencies:
	# Consensus Dependencies
	go get -u github.com/NebulousLabs/demotemutex
	go get -u github.com/NebulousLabs/fastrand
	go get -u github.com/NebulousLabs/merkletree
	go get -u github.com/NebulousLabs/bolt
	go get -u golang.org/x/crypto/blake2b
	go get -u golang.org/x/crypto/ed25519
	# Module + Daemon Dependencies
	go get -u github.com/NebulousLabs/entropy-mnemonics
	go get -u github.com/NebulousLabs/errors
	go get -u github.com/NebulousLabs/go-upnp
	go get -u github.com/NebulousLabs/threadgroup
	go get -u github.com/NebulousLabs/writeaheadlog
	go get -u github.com/klauspost/reedsolomon
	go get -u github.com/julienschmidt/httprouter
	go get -u github.com/inconshreveable/go-update
	go get -u github.com/kardianos/osext
	go get -u github.com/inconshreveable/mousetrap
	# Frontend Dependencies
	go get -u golang.org/x/crypto/ssh/terminal
	go get -u github.com/spf13/cobra/...
	# Developer Dependencies
	go install -race std
	go get -u github.com/client9/misspell/cmd/misspell
	go get -u github.com/golang/lint/golint
	go get -u github.com/NebulousLabs/glyphcheck

# pkgs changes which packages the makefile calls operate on. run changes which
# tests are run during testing.
run = .
pkgs = ./build ./cmd/siac ./cmd/siad ./compatibility ./crypto ./encoding ./modules ./modules/consensus ./modules/explorer \
       ./modules/gateway ./modules/host ./modules/host/contractmanager ./modules/renter ./modules/renter/contractor       \
       ./modules/renter/hostdb ./modules/renter/hostdb/hosttree ./modules/renter/proto ./modules/miner ./modules/wallet   \
       ./modules/transactionpool ./node ./node/api ./persist ./siatest ./node/api/server ./sync ./types

# fmt calls go fmt on all packages.
fmt:
	gofmt -s -l -w $(pkgs)

# vet calls go vet on all packages.
# NOTE: go vet requires packages to be built in order to obtain type info.
vet: release-std
	go vet $(pkgs)

# will always run on some packages for a while.
lintpkgs = ./build ./cmd/siac ./modules ./modules/gateway ./modules/host ./modules/renter ./modules/renter/contractor \
           ./modules/renter/hostdb ./modules/wallet ./node ./node/api/server ./persist ./siatest
lint:
	golint -min_confidence=1.0 -set_exit_status $(lintpkgs)

# spellcheck checks for misspelled words in comments or strings.
spellcheck:
	misspell -error .

# dev builds and installs developer binaries.
dev:
	go install -race -tags='dev debug profile netgo' $(pkgs)

# release builds and installs release binaries.
release:
	go install -tags='debug profile netgo' $(pkgs)
release-race:
	go install -race -tags='debug profile netgo' $(pkgs)
release-std:
	go install -tags 'netgo' -a -ldflags='-s -w' $(pkgs)

# clean removes all directories that get automatically created during
# development.
clean:
	rm -rf release doc/whitepaper.aux doc/whitepaper.log doc/whitepaper.pdf

test:
	go test -short -tags='debug testing netgo' -timeout=5s $(pkgs) -run=$(run)
test-v:
	go test -race -v -short -tags='debug testing netgo' -timeout=15s $(pkgs) -run=$(run)
test-long: clean fmt vet lint
	go test -v -race -tags='testing debug netgo' -timeout=500s $(pkgs) -run=$(run)
test-vlong: clean fmt vet lint
	go test -v -race -tags='testing debug vlong netgo' -timeout=5000s $(pkgs) -run=$(run)
test-cpu:
	go test -v -tags='testing debug netgo' -timeout=500s -cpuprofile cpu.prof $(pkgs) -run=$(run)
test-mem:
	go test -v -tags='testing debug netgo' -timeout=500s -memprofile mem.prof $(pkgs) -run=$(run)
bench: clean fmt
	go test -tags='debug testing netgo' -timeout=500s -run=XXX -bench=$(run) $(pkgs)
cover: clean
	@mkdir -p cover/modules
	@mkdir -p cover/modules/renter
	@mkdir -p cover/modules/host
	@for package in $(pkgs); do                                                                                                 \
		go test -tags='testing debug' -timeout=500s -covermode=atomic -coverprofile=cover/$$package.out ./$$package -run=$(run) \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                                      \
		&& rm cover/$$package.out ;                                                                                             \
	done

# whitepaper builds the whitepaper from whitepaper.tex. pdflatex has to be
# called twice because references will not update correctly the first time.
whitepaper:
	@pdflatex -output-directory=doc whitepaper.tex > /dev/null
	pdflatex -output-directory=doc whitepaper.tex

.PHONY: all dependencies fmt install release release-std xc clean test test-v test-long cover cover-integration cover-unit whitepaper

