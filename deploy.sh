#!/bin/bash

mkdir artifacts
for arch in amd64 arm; do
	for os in darwin linux windows; do
	        for pkg in siac siad; do
			echo $arch/$os
			if [ "$arch" == "arm" ]; then
				if [ "$os" == "windows" ] || [ "$os" == "darwin" ]; then
					continue
				fi
			fi

	                bin=$pkg
	                if [ "$os" == "windows" ]; then
	                        bin=${pkg}.exe
	                fi
	                GOOS=${os} GOARCH=${arch} go build -tags='netgo' -o artifacts/$arch/$os/$bin ./cmd/$pkg
	        done
	done
done
