package ratelimit

import (
	"bytes"
	"testing"
	"time"

	"github.com/NebulousLabs/fastrand"
)

func TestRLSimpleWriteRead(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Set limits
	packetSize := uint64(64)
	bps := int64(1000)
	SetLimits(bps, bps, packetSize)

	// Create a io.ReadWriter.
	rw := bytes.NewBuffer(make([]byte, 0))

	// Wrap it into a rate limited ReadWriter.
	rlc := NewRLReadWriter(rw)

	// Create 1mb to write.
	data := fastrand.Bytes(1000)

	// Write data while measuring time.
	start := time.Now()
	n, err := rlc.Write(data)
	d := time.Since(start)

	// Check for errors
	if n < len(data) {
		t.Error("Not whole data was written")
	}
	if err != nil {
		t.Error("Failed to write data", err)
	}
	// Check the duration. We need to subtract packetSize since the time will
	// be off by one packet. That's because the last written packet will finish
	// faster than anticipated.
	if d.Seconds() < float64(uint64(len(data))-packetSize)/float64(bps) {
		t.Error("Write didn't take long enough", d.Seconds())
	}

	// Read data back from file while measuring time.
	readData := make([]byte, len(data))
	start = time.Now()
	n, err = rlc.Read(readData)
	d = time.Since(start)

	// Check for errors
	if n < len(data) {
		t.Error("Not whole data was read")
	}
	if err != nil {
		t.Error("Failed to read data", err)
	}
	// Check the duration again. Should be the same time.
	if d.Seconds() < float64(uint64(len(data))-packetSize)/float64(bps) {
		t.Error("Read didn't take long enough", d.Seconds())
	}
	// Check if the read data is the same as the written one.
	if bytes.Compare(readData, data) != 0 {
		t.Error("Read data doesn't match written data")
	}
}
