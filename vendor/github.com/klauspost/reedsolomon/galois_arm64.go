//+build !noasm
//+build !appengine

// Copyright 2015, Klaus Post, see LICENSE for details.
// Copyright 2017, Minio, Inc.

package reedsolomon

//go:noescape
func galMulNEON(c uint64, in, out []byte)

//go:noescape
func galMulXorNEON(c uint64, in, out []byte)

func galMulSlice(c byte, in, out []byte, ssse3, avx2 bool) {
	var done int
	galMulNEON(uint64(c), in, out)
	done = (len(in) >> 5) << 5

	remain := len(in) - done
	if remain > 0 {
		mt := mulTable[c]
		for i := done; i < len(in); i++ {
			out[i] = mt[in[i]]
		}
	}
}

func galMulSliceXor(c byte, in, out []byte, ssse3, avx2 bool) {
	var done int
	galMulXorNEON(uint64(c), in, out)
	done = (len(in) >> 5) << 5

	remain := len(in) - done
	if remain > 0 {
		mt := mulTable[c]
		for i := done; i < len(in); i++ {
			out[i] ^= mt[in[i]]
		}
	}
}

// slice galois add
func sliceXor(in, out []byte, sse2 bool) {
	for n, input := range in {
		out[n] ^= input
	}
}
