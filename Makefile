# These variables get inserted into ./build/commit.go
BUILD_TIME=$(shell date)
GIT_REVISION=$(shell git rev-parse --short HEAD)
GIT_DIRTY=$(shell git diff-index --quiet HEAD -- || echo "✗-")

ldflags= -X github.com/NebulousLabs/Sia/build.GitRevision=${GIT_DIRTY}${GIT_REVISION} \
-X "github.com/NebulousLabs/Sia/build.BuildTime=${BUILD_TIME}"

# all will build and install release binaries
all: release

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
	go get -u github.com/NebulousLabs/ratelimit
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
       ./modules/transactionpool ./node ./node/api ./persist ./siatest ./siatest/consensus ./siatest/renter               \
       ./siatest/wallet ./node/api/server ./sync ./types

# fmt calls go fmt on all packages.
fmt:
	gofmt -s -l -w $(pkgs)

# vet calls go vet on all packages.
# NOTE: go vet requires packages to be built in order to obtain type info.
vet: release-std
	go vet $(pkgs)

lint:
	golint -min_confidence=1.0 -set_exit_status $(pkgs)

# spellcheck checks for misspelled words in comments or strings.
spellcheck:
	misspell -error .

# debug builds and installs debug binaries.
debug:
	go install -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
debug-race:
	go install -race -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# dev builds and installs developer binaries.
dev:
	go install -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
dev-race:
	go install -race -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# release builds and installs release binaries.
release:
	go install -tags='netgo' -a -ldflags='-s -w $(ldflags)' $(pkgs)
release-race:
	go install -race -tags='netgo' -a -ldflags='-s -w $(ldflags)' $(pkgs)

# clean removes all directories that get automatically created during
# development.
clean:
	rm -rf cover doc/whitepaper.aux doc/whitepaper.log doc/whitepaper.pdf release 

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
	@mkdir -p cover
	@for package in $(pkgs); do                                                                                                          \
		mkdir -p `dirname cover/$$package`                                                                                        \
		&& go test -tags='testing debug netgo' -timeout=500s -covermode=atomic -coverprofile=cover/$$package.out ./$$package -run=$(run) \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                                               \
		&& rm cover/$$package.out ;                                                                                                      \
	done

# whitepaper builds the whitepaper from whitepaper.tex. pdflatex has to be
# called twice because references will not update correctly the first time.
whitepaper:
	@pdflatex -output-directory=doc whitepaper.tex > /dev/null
	pdflatex -output-directory=doc whitepaper.tex

.PHONY: all dependencies fmt install release release-std xc clean test test-v test-long cover cover-integration cover-unit whitepaper

