#!/bin/bash
set -e

# version and keys are supplied as arguments
version="$1"
keyfile="$2"
pubkeyfile="$3" # optional
if [[ -z $version || -z $keyfile ]]; then
	echo "Usage: $0 VERSION KEYFILE"
	exit 1
fi
if [[ -z $pubkeyfile ]]; then
	echo "Warning: no public keyfile supplied. Binaries will not be verified."
fi

# check for keyfile before proceeding
if [ ! -f $keyfile ]; then
    echo "Key file not found: $keyfile"
    exit 1
fi
keysum=$(shasum -a 256 $keyfile | cut -c -64)
if [ $keysum != "735320b4698010500d230c487e970e12776e88f33ad777ab380a493691dadb1b" ]; then
    echo "Wrong key file: checksum does not match developer key file."
    exit 1
fi

# setup build-time vars
ldflags="-s -w -X 'gitlab.com/NebulousLabs/Sia/build.GitRevision=`git rev-parse --short HEAD`' -X 'gitlab.com/NebulousLabs/Sia/build.BuildTime=`date`'"

for os in darwin linux windows; do
	echo Packaging ${os}...
	# create workspace
	folder=release/Sia-$version-$os-amd64
	rm -rf $folder
	mkdir -p $folder
	# compile and sign binaries
	for pkg in siac siad; do
		bin=$pkg
		if [ "$os" == "windows" ]; then
			bin=${pkg}.exe
		fi
		GOOS=${os} go build -a -tags 'netgo' -ldflags="$ldflags" -o $folder/$bin ./cmd/$pkg
		openssl dgst -sha256 -sign $keyfile -out $folder/${bin}.sig $folder/$bin
		# verify signature
		if [[ -n $pubkeyfile ]]; then
			openssl dgst -sha256 -verify $pubkeyfile -signature $folder/${bin}.sig $folder/$bin
		fi

	done
	# add other artifacts
	cp -r doc LICENSE README.md $folder
	# zip
	(
		cd release
		zip -rq Sia-$version-$os-amd64.zip Sia-$version-$os-amd64
	)
done
