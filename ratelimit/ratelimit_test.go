package ratelimit

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/fastrand"
)

// TestRLConnectionWrites runs multiple tests that check if writing to a
// RLConnection works as expected.
func TestRLConnectionWrites(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Reset bandwidthManager at the end.
	defer func() { BM = nil }()
	// Create server
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	kill := make(chan struct{})
	wait := make(chan struct{})
	defer func() {
		close(kill)
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
	conn, err := (&net.Dialer{}).Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Specifiy subtests to run
	subTests := []struct {
		name string
		test func(net.Conn, int, *testing.T)
	}{
		{"TestSingleWrite", testSingleWrite},
		{"TestMultipleWrites", testMultipleWrites},
	}

	// Configure rate limit
	packetSize := int64(250)
	pps := int64(4)
	dataLen := 1000
	stop := make(chan struct{})
	defer close(stop)
	Init(pps, 0, packetSize, stop)
	client := NewRLConn(conn)
	defer client.Close()
	// Run tests
	for _, test := range subTests {
		t.Run(test.name, func(t *testing.T) {
			start := time.Now()
			test.test(client, dataLen, t)
			d := time.Since(start)

			// It should have taken at least dataLen / (packetSize * packetsPerSecond)
			// seconds.
			if d.Seconds() < float64(dataLen)/float64(packetSize*pps) {
				t.Fatalf("Transmission finished too soon. %v seconds.", d.Seconds())
			}
		})
	}
}

// TestRLConnectionReads runs multiple tests that check if reading from a
// RLConnection works as expected.
func TestRLConnectionReads(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Reset bandwidthManager at the end.
	defer func() { BM = nil }()
	// Create server
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	serverChan := make(chan net.Conn)
	go func() {
		server, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		serverChan <- server
	}()

	// Create client
	wait := make(chan struct{})
	defer func() {
		<-wait
	}()
	go func() {
		defer close(wait)
		conn, err := (&net.Dialer{}).Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		// Write 1 mb
		_, _ = conn.Write(fastrand.Bytes(1 * 1 << 20))
	}()

	// Specifiy subtests to run
	subTests := []struct {
		name string
		test func(net.Conn, int, *testing.T)
	}{
		{"TestSingleRead", testSingleRead},
		{"TestMultipleReads", testMultipleReads},
	}

	// Configure rate limit
	packetSize := int64(250)
	pps := int64(4)
	dataLen := 1000
	stop := make(chan struct{})
	defer close(stop)
	Init(0, pps, packetSize, stop)
	server := NewRLConn(<-serverChan)
	defer server.Close()
	// Run tests
	for _, test := range subTests {
		t.Run(test.name, func(t *testing.T) {
			start := time.Now()
			test.test(server, dataLen, t)
			d := time.Since(start)

			// It should have taken at least dataLen / (packetSize * packetsPerSecond)
			// seconds.
			if d.Seconds() < float64(dataLen)/float64(packetSize*pps) {
				t.Fatalf("Transmission finished too soon. %v seconds.", d.Seconds())
			}
		})
	}
}

// testSingleRead tests if a single rate-limited read works as expected.
func testSingleRead(server net.Conn, dataLen int, t *testing.T) {
	dataReceived := make([]byte, dataLen)

	n, err := server.Read(dataReceived)
	if err != nil {
		t.Fatal(err)
	}
	if n != dataLen {
		t.Fatal("didn't receive enough data")
	}
}

// testMultipleReads tests if multiple rate-limited read work as expected.
func testMultipleReads(server net.Conn, dataLen int, t *testing.T) {
	maxReadLen := 10

	// Create slice to read into
	data := make([]byte, dataLen)

	readData := 0
	for readData < dataLen {
		// Randomly decide how much data to read during this iteration
		toRead := fastrand.Intn(maxReadLen) + 1
		if readData+toRead > dataLen {
			toRead = dataLen - readData
		}
		// Read data
		n, err := server.Read(data[readData : readData+toRead])
		if err != nil {
			t.Fatal(err)
		}
		readData += n
	}
}

// testSingleWrite tests if a single rate-limited write works as expected.
func testSingleWrite(client net.Conn, dataLen int, t *testing.T) {
	// Create data to send.
	data := fastrand.Bytes(dataLen)

	// Write data
	n, err := client.Write(data)
	if err != nil {
		t.Error(err)
	}
	if n != len(data) {
		t.Error("Not all data was written")
	}
}

// testMultipleWrites tests if a multiple rate-limited writes works as expected.
func testMultipleWrites(client net.Conn, dataLen int, t *testing.T) {
	maxWriteLen := 10

	// Create data to send.
	data := fastrand.Bytes(dataLen)

	// Write data
	writtenData := 0
	for writtenData < dataLen {
		// Randomly decide how much data to write during this iteration
		toWrite := fastrand.Intn(maxWriteLen) + 1
		if writtenData+toWrite > dataLen {
			toWrite = dataLen - writtenData
		}
		// Write data
		n, err := client.Write(data[writtenData : writtenData+toWrite])
		if err != nil {
			t.Error(err)
		}
		if n != toWrite {
			t.Error("written data != toWrite")
		}
		writtenData += n
	}
}
