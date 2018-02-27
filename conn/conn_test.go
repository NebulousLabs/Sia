package conn

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/fastrand"
)

// TestRLConnectionSingleWrite tests a single write to a RLConnection.
func TestRLConnectionSingleWrite(t *testing.T) {
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
	client, err := (&net.Dialer{}).Dial("tcp", ":1234")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Run tests
	tests := []func(client net.Conn, t *testing.T){
		testSingleWrite,
	}
	for _, test := range tests {
		test(client, t)
	}
}

// testSingleWrite tests if a single rate-limited write works as expected.
func testSingleWrite(conn net.Conn, t *testing.T) {
	// Set limit
	packetSize := int64(250)
	packetsPerSecond := int64(4)
	client := newRLConn(conn, packetSize, packetsPerSecond)

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
