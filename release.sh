#!/bin/bash
set -e

# version and private key are supplied as arguments
version="$1"
keyfile="$2"
if [[ -z $version || -z $keyfile ]]; then
	echo "Usage: $0 VERSION KEYFILE"
	exit 1
fi

# check for keyfile before proceeding
if [ ! -f $keyfile ]; then
    echo "Key file not found: $keyfile"
    exit 1
fi

for os in darwin linux windows; do
	echo Packaging ${os}...
	# create workspace
	root=$(pwd)
	folder=$root/release/Sia-$version-$os-amd64
	rm -rf $folder
	mkdir -p $folder
	# compile and sign binaries
	for pkg in siac siad; do
		bin=$pkg
		if [ "$os" == "windows" ]; then
			bin=${pkg}.exe
		fi
		GOOS=${os} go build -o $folder/$bin ./$pkg
		openssl dgst -sha256 -sign $keyfile -out $folder/${bin}.sig $folder/$bin
	done
	# add other artifacts
	cp -r $root/doc $root/LICENSE $root/README.md $folder
	# zip
	zip -rq release/Sia-$version-$os-amd64.zip $folder
done
