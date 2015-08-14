package profile

import (
	"fmt"
	"time"
)

var (
	times        map[string]int64
	activeTimers map[string]int64
)

// PrintTimes prints how much time has passed at each timer.
func PrintTimes() string {
	s := "Printing Timers:\n"
	for name, time := range times {
		s += fmt.Sprintf("\t%v: %v\n", name, time)
	}
	return s
}

// ToggleTimer actives a timer known by a given string. If the timer does not
// yet exist, it is created.
func ToggleTimer(s string) {
	if times == nil {
		times = make(map[string]int64)
		activeTimers = make(map[string]int64)
	}

	toggleTime, exists := activeTimers[s]
	if exists {
		times[s] = times[s] + (time.Now().UnixNano() - toggleTime)
		delete(activeTimers, s)
	} else {
		activeTimers[s] = time.Now().UnixNano()
	}
}
