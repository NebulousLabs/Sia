package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"

	"gitlab.com/NebulousLabs/Sia/persist"
)

// There's a global lock on cpu and memory profiling, because I'm not sure what
// happens if multiple threads call each at the same time. This lock might be
// unnecessary.
var (
	cpuActive   bool
	cpuLock     sync.Mutex
	memActive   bool
	memLock     sync.Mutex
	traceActive bool
	traceLock   sync.Mutex
)

// StartCPUProfile starts cpu profiling. An error will be returned if a cpu
// profiler is already running.
func StartCPUProfile(profileDir, identifier string) error {
	// Lock the cpu profile lock so that only one profiler is running at a
	// time.
	cpuLock.Lock()
	if cpuActive {
		cpuLock.Unlock()
		return errors.New("cannot start cpu profiler, a profiler is already running")
	}
	cpuActive = true
	cpuLock.Unlock()

	// Start profiling into the profile dir, using the identifer. The timestamp
	// of the start time of the profiling will be included in the filename.
	cpuProfileFile, err := os.Create(filepath.Join(profileDir, "cpu-profile-"+identifier+"-"+time.Now().Format(time.RFC3339Nano)+".prof"))
	if err != nil {
		return err
	}
	pprof.StartCPUProfile(cpuProfileFile)
	return nil
}

// StopCPUProfile stops cpu profiling.
func StopCPUProfile() {
	cpuLock.Lock()
	if cpuActive {
		pprof.StopCPUProfile()
		cpuActive = false
	}
	cpuLock.Unlock()
}

// SaveMemProfile saves the current memory structure of the program. An error
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

// StartTrace starts trace. An error will be returned if a trace
// is already running.
func StartTrace(traceDir, identifier string) error {
	// Lock the trace lock so that only one profiler is running at a
	// time.
	traceLock.Lock()
	if traceActive {
		traceLock.Unlock()
		return errors.New("cannot start trace, it is already running")
	}
	traceActive = true
	traceLock.Unlock()

	// Start trace into the trace dir, using the identifer. The timestamp
	// of the start time of the trace will be included in the filename.
	traceFile, err := os.Create(filepath.Join(traceDir, "trace-"+identifier+"-"+time.Now().Format(time.RFC3339Nano)+".trace"))
	if err != nil {
		return err
	}
	return trace.Start(traceFile)
}

// StopTrace stops trace.
func StopTrace() {
	traceLock.Lock()
	if traceActive {
		trace.Stop()
		traceActive = false
	}
	traceLock.Unlock()
}

// startContinuousLog creates dir and saves inexpensive logs periodically.
// It also runs the restart function periodically.
func startContinuousLog(dir string, sleepCap time.Duration, restart func()) {
	// Create the folder for all of the profiling results.
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		fmt.Println(err)
		return
	}
	// Continuously log statistics about the running Sia application.
	go func() {
		// Create the logger.
		log, err := persist.NewFileLogger(filepath.Join(dir, "continuousStats.log"))
		if err != nil {
			fmt.Println("Stats logging failed:", err)
			return
		}
		// Collect statistics in an infinite loop.
		sleepTime := time.Second * 10
		for {
			// Sleep for an exponential amount of time each iteration, this
			// keeps the size of the log small while still providing lots of
			// information.
			restart()
			time.Sleep(sleepTime)
			sleepTime = time.Duration(1.2 * float64(sleepTime))
			if sleepCap != 0*time.Second && sleepTime > sleepCap {
				sleepTime = sleepCap
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Printf("\n\tGoroutines: %v\n\tAlloc: %v\n\tTotalAlloc: %v\n\tHeapAlloc: %v\n\tHeapSys: %v\n", runtime.NumGoroutine(), m.Alloc, m.TotalAlloc, m.HeapAlloc, m.HeapSys)
		}
	}()
}

// StartContinuousProfile will continuously print statistics about the cpu
// usage, memory usage, and runtime stats of the program, and run an execution
// logger. Select one (recommended) or more functionalities by passing the
// corresponding flag(s)
func StartContinuousProfile(profileDir string, profileCPU bool, profileMem bool, profileTrace bool) {
	sleepCap := 0 * time.Second // Unlimited.
	if profileTrace {
		sleepCap = 10 * time.Minute
	}
	startContinuousLog(profileDir, sleepCap, func() {
		if profileCPU {
			StopCPUProfile()
			StartCPUProfile(profileDir, "continuousProfileCPU")
		}
		if profileMem {
			SaveMemProfile(profileDir, "continuousProfileMem")
		}
		if profileTrace {
			StopTrace()
			StartTrace(profileDir, "continuousProfileTrace")
		}
	})
}
