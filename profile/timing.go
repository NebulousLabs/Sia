package profile

import (
	"fmt"
	"time"
)

var (
	uptime       int64
	times        = make(map[string]int64)
	activeTimers = make(map[string]int64)
)

// Uptime() returns the number of nanoseconds that have passed since the first
// call to uptime.
func Uptime() int64 {
	if uptime == 0 {
		uptime = time.Now().UnixNano()
		return 0
	}
	return (time.Now().UnixNano() - uptime) / 1e6
}

// PrintTimes prints how much time has passed at each timer.
func PrintTimes() string {
	s := "Printing Timers:\n"
	for name, time := range times {
		s += fmt.Sprintf("\t%v: %v\n", name, time/1e6)
	}
	return s
}

// ToggleTimer actives a timer known by a given string. If the timer does not
// yet exist, it is created.
func ToggleTimer(s string) {
	toggleTime, exists := activeTimers[s]
	if exists {
		times[s] = times[s] + (time.Now().UnixNano() - toggleTime)
		delete(activeTimers, s)
	} else {
		activeTimers[s] = time.Now().UnixNano()
	}
}
