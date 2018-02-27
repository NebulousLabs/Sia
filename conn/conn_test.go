package conn

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

// TestRLConnectionReads runs multiple tests that check if reading from a
// RLConnection works as expected.
func TestRLConnectionReads(t *testing.T) {
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

		// Write 100 mb
		_, _ = conn.Write(fastrand.Bytes(100 * 1 << 20))
	}()

	// Specifiy subtests to run
	subTests := []struct {
		name string
		test func(net.Conn, *testing.T)
	}{
		{"TestSingleRead", testSingleRead},
		{"TestMultipleReads", testMultipleReads},
	}

	// Get server conn
	conn := <-serverChan
	defer conn.Close()

	// Run tests
	for _, test := range subTests {
		t.Run(test.name, func(t *testing.T) {
			test.test(conn, t)
		})
	}
}

// testSingleRead tests if a single rate-limited read works as expected.
func testSingleRead(conn net.Conn, t *testing.T) {
	// Set limit
	packetSize := int64(250)
	packetsPerSecond := int64(4)
	server := NewRLConn(conn, packetSize, packetsPerSecond, 0, 0)
	dataLen := 1000
	dataReceived := make([]byte, dataLen)

	// Read data and see how long it takes
	start := time.Now()
	n, err := server.Read(dataReceived)
	if err != nil {
		t.Fatal(err)
	}
	if n != dataLen {
		t.Fatal("didn't receive enough data")
	}
	d := time.Since(start)

	// It should have taken at least len(dataReceived) / (packetSize * packetsPerSecond)
	// seconds.
	if d.Seconds() < float64(dataLen)/float64(packetSize*packetsPerSecond) {
		t.Fatalf("Transmission finished too soon. %v seconds.", d.Seconds())
	}
}

// testMultipleReads tests if multiple rate-limited read work as expected.
func testMultipleReads(conn net.Conn, t *testing.T) {
	// Set limit
	packetSize := int64(250)
	packetsPerSecond := int64(4)
	server := NewRLConn(conn, packetSize, packetsPerSecond, 0, 0)
	dataLen := 1000
	dataReceived := make([]byte, dataLen)

	// Read data and see how long it takes
	readData := 0
	start := time.Now()
	for readData < dataLen {
		n, err := server.Read(dataReceived[readData:])
		if err != nil {
			t.Fatal(err)
		}
		readData += n
	}
	d := time.Since(start)

	// It should have taken at least len(dataReceived) / (packetSize * packetsPerSecond)
	// seconds.
	if d.Seconds() < float64(dataLen)/float64(packetSize*packetsPerSecond) {
		t.Fatalf("Transmission finished too soon. %v seconds.", d.Seconds())
	}
}

// testSingleWrite tests if a single rate-limited write works as expected.
func testSingleWrite(conn net.Conn, t *testing.T) {
	// Set limit
	packetSize := int64(250)
	packetsPerSecond := int64(4)
	client := NewRLConn(conn, 0, 0, packetSize, packetsPerSecond)

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
	client := NewRLConn(conn, 0, 0, packetSize, packetsPerSecond)

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
