# all will build and install developer binaries, which have debugging enabled
# and much faster mining and block constants.
all: install

# dependencies installs all of the dependencies that are required for building
# Sia.
dependencies:
	go install -race std
	go get -u github.com/NebulousLabs/ed25519
	go get -u github.com/NebulousLabs/entropy-mnemonics
	go get -u github.com/NebulousLabs/go-upnp
	go get -u github.com/NebulousLabs/merkletree
	go get -u github.com/bgentry/speakeasy
	go get -u github.com/boltdb/bolt
	go get -u github.com/dchest/blake2b
	go get -u github.com/inconshreveable/go-update
	go get -u github.com/inconshreveable/muxado
	go get -u github.com/kardianos/osext
	go get -u github.com/klauspost/reedsolomon
	go get -u github.com/laher/goxc
	go get -u github.com/spf13/cobra
	go get -u github.com/stretchr/graceful
	go get -u golang.org/x/crypto/twofish
	go get -u golang.org/x/tools/cmd/cover

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
	go install -race -tags='dev debug profile' ./...

# release builds and installs release binaries.
release: REBUILD
	go install -a -race -tags='debug profile' ./...
release-std: REBUILD
	go install -a ./...

# xc builds and packages release binaries for all systems by using goxc.
# Cross Compile - makes binaries for windows, linux, and mac, 32 and 64 bit.
xc: dependencies test test-long REBUILD
	goxc -arch="386 amd64 arm" -bc="linux windows darwin" -d=release -pv=0.4.0   \
	     -br=release -pr=beta -include=LICENSE,README.md,doc/API.md              \
	     -main-dirs-exclude=siae,siag -tasks-=deb,deb-dev,deb-source,go-test     \
	     -n=Sia
xc-siag: dependencies test test-long REBUILD
	goxc -arch="386 amd64 arm" -bc="linux windows darwin" -d=release -pv=1.0.1    \
	     -br=release -include=LICENSE                                             \
	     -main-dirs-exclude=siac,siad,siae -tasks-=deb,deb-dev,deb-source,go-test \
	     -n=Sia_Address_Generator

# clean removes all directories that get automatically created during
# development.
clean:
	rm -rf release doc/whitepaper.aux doc/whitepaper.log doc/whitepaper.pdf

# 3 commands and a variable are available for testing Sia packages. 'pkgs'
# indicates which packages should be tested, and defaults to all the packages
# with test files. Using './...' as default breaks compatibility with the cover
# command. 'test' runs short tests that should last no more than a few seconds,
# 'test-long' runs more thorough tests which should not last more than a few
# minutes.
pkgs = ./api ./compatibility ./crypto ./encoding ./modules/consensus \
       ./modules/gateway ./modules/host ./modules/hostdb		     \
       ./modules/miner ./modules/renter ./modules/transactionpool    \
       ./modules/wallet ./modules/explorer ./persist                 \
       ./siag ./siae ./types
test: clean fmt REBUILD
	go test -short -tags='debug testing' -timeout=10s $(pkgs)
test-v: clean fmt REBUILD
	go test -race -v -short -tags='debug testing' -timeout=35s $(pkgs)
test-long: clean fmt REBUILD
	go test -v -race -tags='testing debug' -timeout=300s $(pkgs)
bench: clean fmt REBUILD
	go test -tags='testing' -timeout=300s -run=XXX -bench=. $(pkgs)
cover: clean REBUILD
	@mkdir -p cover/modules
	@for package in $(pkgs); do                                                                                     \
		go test -tags='testing debug' -timeout=360s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done
cover-integration: clean REBUILD
	@mkdir -p cover/modules
	@for package in $(pkgs); do                                                                                     \
		go test -run=TestIntegration -tags='testing debug' -timeout=300s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done
cover-unit: clean REBUILD
	@mkdir -p cover/modules
	@for package in $(pkgs); do                                                                                     \
		go test -run=TestUnit -tags='testing debug' -timeout=300s -covermode=atomic -coverprofile=cover/$$package.out ./$$package \
		&& go tool cover -html=cover/$$package.out -o=cover/$$package.html                                          \
		&& rm cover/$$package.out ;                                                                                 \
	done

# whitepaper builds the whitepaper from whitepaper.tex. pdflatex has to be
# called twice because references will not update correctly the first time.
whitepaper:
	@pdflatex -output-directory=doc whitepaper.tex > /dev/null
	pdflatex -output-directory=doc whitepaper.tex

.PHONY: all dependencies fmt REBUILD install release xc clean test test-long cover whitepaper
