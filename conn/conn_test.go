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
	// Create server side
	ln, err := net.Listen("tcp", ":1234")
	dataLen := 1000
	data := fastrand.Bytes(dataLen)
	wait := make(chan struct{})
	go func() {
		defer close(wait)
		server, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
		readBytes := 0
		buf := make([]byte, dataLen)
		for readBytes < dataLen {
			n, err := io.ReadFull(server, buf)
			if err != nil {
				t.Fatal(err)
			}
			readBytes += n
		}
	}()

	client, err := Dial(&net.Dialer{}, "tcp", ":1234")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Set limit
	packetSize := uint64(250)
	packetsPerSecond := uint64(1)
	client.SetLimit(packetSize, packetsPerSecond)

	// Write data and time how long it takes.
	start := time.Now()
	_, err = client.Write(data)
	if err != nil {
		t.Error(err)
	}
	d := time.Since(start)

	// It should have taken at least dataLen / (packetSize * packetsPerSecond)
	// seconds.
	if d.Seconds() < float64(dataLen)/float64(packetSize*packetsPerSecond) {
		t.Errorf("Transmission finished too soon. %v seconds.", d.Seconds())
	}

	// Wait for server to stop
	<-wait
}
