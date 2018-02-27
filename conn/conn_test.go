package conn

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/fastrand"
)

// TestRLConnectionWrites runs multiple tests that test if writing to a
// RLConnection works as expected.
func TestRLConnectionWrites(t *testing.T) {
	// Create server
	ln, err := net.Listen("tcp", ":1234")
	kill := make(chan struct{})
	wait := make(chan struct{})
	defer func() {
		defer close(kill)
		<-wait
	}()
	go func() {
		defer close(wait)
		server, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
		buf := make([]byte, 100)
		for {
			select {
			case <-kill:
				break
			default:
			}
			_, err := server.Read(buf)
			if err != nil && err != io.EOF {
				t.Fatal(err)
			}
			if err == io.EOF {
				break
			}
		}
	}()

	// Create client
	conn, err := (&net.Dialer{}).Dial("tcp", ":1234")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Specifiy subtests to run
	subTests := []struct {
		name string
		test func(net.Conn, *testing.T)
	}{
		{"TestSingleWrite", testSingleWrite},
		{"TestMultipleWrites", testMultipleWrites},
	}

	// Run tests
	for _, test := range subTests {
		t.Run(test.name, func(t *testing.T) {
			test.test(conn, t)
		})
	}
}

// testSingleWrite tests if a single rate-limited write works as expected.
func testSingleWrite(conn net.Conn, t *testing.T) {
	// Set limit
	packetSize := int64(250)
	packetsPerSecond := int64(4)
	client := NewRLConn(conn, packetSize, packetsPerSecond)

	// Create data to send.
	dataLen := 1000
	data := fastrand.Bytes(dataLen)

	// Write data and time how long it takes.
	start := time.Now()
	_, err := client.Write(data)
	if err != nil {
		t.Error(err)
	}
	d := time.Since(start)
	// It should have taken at least dataLen / (packetSize * packetsPerSecond)
	// seconds.
	if d.Seconds() < float64(dataLen)/float64(packetSize*packetsPerSecond) {
		t.Fatalf("Transmission finished too soon. %v seconds.", d.Seconds())
	}
}

// testMultipleWrites tests if a multiple rate-limited writes works as expected.
func testMultipleWrites(conn net.Conn, t *testing.T) {
	// Set limit
	packetSize := int64(250)
	packetsPerSecond := int64(4)
	maxWriteLen := 10
	client := NewRLConn(conn, packetSize, packetsPerSecond)

	// Create data to send.
	dataLen := 1000
	data := fastrand.Bytes(dataLen)

	// Write data and time how long it takes.
	writtenData := 0
	start := time.Now()
	for writtenData < dataLen {
		// Randomly decide how much data to write during this iteration
		toWrite := fastrand.Intn(maxWriteLen) + 1
		if writtenData+toWrite > dataLen {
			toWrite = dataLen - writtenData
		}
		// Write data
		n, err := client.Write(data[writtenData:])
		if err != nil {
			t.Error(err)
		}
		writtenData += n
	}
	d := time.Since(start)
	// It should have taken at least dataLen / (packetSize * packetsPerSecond)
	// seconds.
	if d.Seconds() < float64(dataLen)/float64(packetSize*packetsPerSecond) {
		t.Fatalf("Transmission finished too soon. %v seconds.", d.Seconds())
	}
}
