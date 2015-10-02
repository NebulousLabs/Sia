package consensus

import (
	"strconv"
	"testing"
)

// BenchmarkCreateServerTester benchmarks creating a server tester from
// scratch. The consensus package creates over 60 server testers (and
// counting), and optimizations to the server tester creation process are
// likely to generalize to the project as a whole.
//
// i7-4770, 1d60d69: 22.883 ms / op
func BenchmarkCreateServerTester(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cst, err := createConsensusSetTester("BenchmarkCreateServerTester - " + strconv.Itoa(i))
		if err != nil {
			b.Fatal(err)
		}
		cst.closeCst()
	}
}
