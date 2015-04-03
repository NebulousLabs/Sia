# all will build and install developer binaries, which have debugging enabled
# and much faster mining and block constants.
all: install

# dependencies installs all of the dependencies that are required for building
# Sia.
dependencies:
	go install -race std
	go get -u code.google.com/p/gcfg
	go get -u github.com/agl/ed25519
	go get -u github.com/dchest/blake2b
	go get -u github.com/inconshreveable/go-update
	go get -u github.com/laher/goxc
	go get -u github.com/mitchellh/go-homedir
	go get -u github.com/spf13/cobra
	go get -u github.com/stretchr/graceful
	go get -u golang.org/x/crypto/twofish
	go get -u golang.org/x/tools/cmd/cover
	go get -u github.com/NebulousLabs/merkletree

# fmt calls go fmt on all packages.
fmt:
	go fmt ./...

# REBUILD touches all of the build-dependent source files, forcing them to be
# rebuilt. This is necessary because the go tool is not smart enough to trigger
# a rebuild when build tags have been changed.
REBUILD:
	@touch build/*.go

# install builds and installs developer binaries.
install: fmt REBUILD
	go install -tags='dev debug' ./...

# release builds and installs release binaries.
release: dependencies test-long REBUILD
	go install ./...

# xc builds and packages release binaries for all systems by using goxc.
# Cross Compile - makes binaries for windows, linux, and mac, 32 and 64 bit.
xc: dependencies test test-long REBUILD
	goxc -arch="amd64" -bc="linux windows darwin" -d=release -pv=0.3.0          \
		-br=release -pr=beta -include=example-config,LICENSE*,README*,API*      \
		-tasks-=deb,deb-dev,deb-source,go-test

# clean removes all directories that get automatically created during
# development.
clean:
	rm -rf hostdir release whitepaper.aux whitepaper.log whitepaper.pdf         \
		*.wallet* *_test testdir* */*_test hostdir* siad/walletDir*             \
		siad/hostDir* info.log modules/*/*.dat modules/*/*.backup               \
		/tmp/SiaTesting

# test runs the short tests for Sia, and aims to always take less than 2
# seconds.
test: clean fmt REBUILD
	go test -short -tags='debug testing' -timeout=1s ./...

# test-long does a forced rebuild of all packages, and then runs all tests
# with the race libraries enabled. test-long aims to be
# thorough.
test-long: clean fmt REBUILD
	go test -v -race -tags='testing debug' -timeout=180s ./...

# Testing for each package individually. Packages are added to this set as needed.
test-tpool: clean fmt REBUILD
	go test -v -race -tags='testing debug' -timeout=8s ./modules/transactionpool
test-wallet: clean fmt REBUILD
	go test -v -race -tags='testing debug' -timeout=8s ./modules/wallet

# cover runs the long tests and creats html files that show you which lines
# have been hit during testing and how many times each line has been hit.
coverpackages = api crypto encoding modules/consensus modules/gateway           \
	modules/host modules/hostdb modules/miner modules/renter                    \
	modules/transactionpool modules/wallet types
cover: clean REBUILD
	@mkdir -p cover/modules
	@for package in $(coverpackages); do \
		go test -tags='testing debug' -covermode=atomic -coverprofile=cover/$$package.out ./$$package ; \
		go tool cover -html=cover/$$package.out -o=cover/$$package.html ; \
		rm cover/$$package.out ; \
	done

# whitepaper builds the whitepaper from whitepaper.tex. pdflatex has to be
# called twice because references will not update correctly the first time.
whitepaper:
	@pdflatex whitepaper.tex > /dev/null
	pdflatex whitepaper.tex

.PHONY: all fmt install clean test test-long cover whitepaper dependencies release xc REBUILD
