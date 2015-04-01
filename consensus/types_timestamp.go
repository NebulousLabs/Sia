package consensus

import (
	"time"
)

type (
	Timestamp      uint64
	TimestampSlice []Timestamp
)

func (ts TimestampSlice) Len() int {
	return len(ts)
}

func (ts TimestampSlice) Less(i, j int) bool {
	return ts[i] < ts[j]
}

func (ts TimestampSlice) Swap(i, j int) {
	ts[i], ts[j] = ts[j], ts[i]
}

// CurrentTimestamp returns the current time as a Timestamp.
func CurrentTimestamp() Timestamp {
	return Timestamp(time.Now().Unix())
}
