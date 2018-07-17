package siatest

import "gitlab.com/NebulousLabs/fastrand"

// Fuzz returns 0, 1 or -1. This can be used to test for random off-by-one
// errors in the code. For example fuzz can be used to create a File that is
// either sector aligned or off-by-one.
func Fuzz() int {
	// Intn(3) creates a number of the set [0,1,2]. By subtracting 1 we end up
	// with a number of the set [-1,0,1].
	return fastrand.Intn(3) - 1
}
