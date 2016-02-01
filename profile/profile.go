package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/persist"
)

// There's a global lock on cpu and memory profiling, because I'm not sure what
// happens if multiple threads call each at the same time. This lock might be
// unnecessary.
var (
	cpuActive bool
	cpuLock   sync.Mutex
	memActive bool
	memLock   sync.Mutex
)

// startCPUProfile starts cpu profiling. An error will be returned if a cpu
// profiler is already running.
func StartCPUProfile(profileDir, identifier string) error {
	// Lock the cpu profile lock so that only one profiler is running at a
	// time.
	cpuLock.Lock()
	if cpuActive {
		cpuLock.Unlock()
		return errors.New("cannot start cpu profilier, a profiler is already running")
	}
	cpuActive = true
	cpuLock.Unlock()

	// Start profiling into the profile dir, using the identifer. The timestamp
	// of the start time of the profiling will be included in the filenmae.
	cpuProfileFile, err := os.Create(filepath.Join(profileDir, "cpu-profile-"+identifier+"-"+time.Now().Format(time.RFC3339Nano)+".prof"))
	if err != nil {
		return err
	}
	pprof.StartCPUProfile(cpuProfileFile)
	return nil
}

// stopCPUProfile stops cpu profiling.
func StopCPUProfile() {
	cpuLock.Lock()
	if cpuActive {
		pprof.StopCPUProfile()
		cpuActive = false
	}
	cpuLock.Unlock()
}

// saveMemProfile saves the current memory structure of the program. An error
// will be returned if memory profiling is already in progress. Unlike for cpu
// profiling, there is no 'stopMemProfile' call - everything happens at once.
func SaveMemProfile(profileDir, identifier string) error {
	memLock.Lock()
	if memActive {
		memLock.Unlock()
		return errors.New("cannot start memory profiler, a memory profiler is already running")
	}
	memActive = true
	memLock.Unlock()

	// Save the memory profile.
	memFile, err := os.Create(filepath.Join(profileDir, "mem-profile-"+identifier+"-"+time.Now().Format(time.RFC3339Nano)+".prof"))
	if err != nil {
		return err
	}
	pprof.WriteHeapProfile(memFile)

	memLock.Lock()
	memActive = false
	memLock.Unlock()
	return nil
}

// StartContinuousProfiling will continuously print statistics about the cpu
// usage, memory usage, and runtime stats of the program.
func StartContinuousProfile(profileDir string) {
	// Create the folder for all of the profiling results.
	err := os.MkdirAll(profileDir, 0700)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Continuously log statistics about the running Sia application.
	go func() {
		// Create the logger.
		log, err := persist.NewLogger(filepath.Join(profileDir, "continuousProfiling.log"))
		if err != nil {
			fmt.Println("Profile logging failed:", err)
			return
		}

		// Collect statistics in an infinite loop.
		sleepTime := time.Second * 20
		for {
			// Sleep for an exponential amount of time each iteration, this
			// keeps the size of the log small while still providing lots of
			// information.
			time.Sleep(sleepTime)
			sleepTime = time.Duration(1.5 * float64(sleepTime))

			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Printf("\n\tGoroutines: %v\n\tAlloc: %v\n\tTotalAlloc: %v\n\tHeapAlloc: %v\n\tHeapSys: %v\n", runtime.NumGoroutine(), m.Alloc, m.TotalAlloc, m.HeapAlloc, m.HeapSys)
		}
	}()
}
