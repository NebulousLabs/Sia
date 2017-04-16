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

	"github.com/NebulousLabs/Sia/persist"
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
		return errors.New("cannot start cpu profilier, a profiler is already running")
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

func startContinuousLog(dir string, restart func()) {
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
		sleepTime := time.Second * 20
		for {
			// Sleep for an exponential amount of time each iteration, this
			// keeps the size of the log small while still providing lots of
			// information.
			restart()
			time.Sleep(sleepTime)
			sleepTime = time.Duration(1.5 * float64(sleepTime))

			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Printf("\n\tGoroutines: %v\n\tAlloc: %v\n\tTotalAlloc: %v\n\tHeapAlloc: %v\n\tHeapSys: %v\n", runtime.NumGoroutine(), m.Alloc, m.TotalAlloc, m.HeapAlloc, m.HeapSys)
		}
	}()
}

// StartContinuousProfiling will continuously print statistics about the cpu
// usage, memory usage, and runtime stats of the program.
func StartContinuousProfile(profileDir string) {
	startContinuousLog(profileDir, func() {
		StopCPUProfile()
		SaveMemProfile(profileDir, "continuousProfilingMem")
		StartCPUProfile(profileDir, "continuousProfilingCPU")
	})
}

// StartContinuousTrace will continuously run execution logger.
func StartContinuousTrace(traceDir string) {
	startContinuousLog(traceDir, func() {
		StopTrace()
		StartTrace(traceDir, "continuousTrace")
	})
}
