package persist

// disk_test.go probes some of the disk operations that are very commonly used
// within Sia. Namely, Read, Write, Truncate, WriteAt(rand), ReadAt(rand).

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/NebulousLabs/Sia/build"

	"gitlab.com/NebulousLabs/fastrand"
)

// BenchmarkWrite512MiB checks how long it takes to write 512MiB sequentially.
func BenchmarkWrite512MiB(b *testing.B) {
	testDir := build.TempDir("persist", b.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(1 << 29)
	filename := filepath.Join(testDir, "512MiB.file")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make the file.
		f, err := os.Create(filename)
		if err != nil {
			b.Fatal(err)
		}

		// 2^12 writes of 4MiB.
		for i := 0; i < 1<<7; i++ {
			// Get the entropy separate from the timer.
			b.StopTimer()
			data := fastrand.Bytes(1 << 22)
			b.StartTimer()
			_, err = f.Write(data)
			if err != nil {
				b.Fatal(err)
			}
			// Sync after every write.
			err = f.Sync()
			if err != nil {
				b.Fatal(err)
			}
		}

		// Close the file before iterating.
		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	err = os.Remove(filename)
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkWrite512MiBTrunc checks how long it takes to write 512MiB using
// stepwise truncate.
func BenchmarkWrite512MiBTrunc(b *testing.B) {
	testDir := build.TempDir("persist", b.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(1 << 29)
	filename := filepath.Join(testDir, "512MiB.file")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make the file.
		f, err := os.Create(filename)
		if err != nil {
			b.Fatal(err)
		}

		// 2^12 writes of 4MiB.
		for i := 0; i < 1<<7; i++ {
			// Extend the file through truncation.
			err = f.Truncate(int64((i + 1) * 1 << 22))
			if err != nil {
				b.Fatal(err)
			}
			// Sync after every write.
			err = f.Sync()
			if err != nil {
				b.Fatal(err)
			}
		}

		// Close the file before iterating.
		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	err = os.Remove(filename)
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkWrite512MiBRand checks how long it takes to write 512MiB randomly.
func BenchmarkWrite512MiBRand(b *testing.B) {
	testDir := build.TempDir("persist", b.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(1 << 29)
	filename := filepath.Join(testDir, "512MiB.file")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make the file.
		f, err := os.Create(filename)
		if err != nil {
			b.Fatal(err)
		}

		// 2^7 writes of 4MiB.
		for i := 0; i < 1<<7; i++ {
			// Get the entropy separate from the timer.
			b.StopTimer()
			data := fastrand.Bytes(1 << 22)
			offset := int64(fastrand.Intn(1 << 6))
			offset *= 1 << 22
			b.StartTimer()
			_, err = f.WriteAt(data, offset)
			if err != nil {
				b.Fatal(err)
			}
			// Sync after every write.
			err = f.Sync()
			if err != nil {
				b.Fatal(err)
			}
		}

		// Close the file before iterating.
		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	err = os.Remove(filename)
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkRead512MiB checks how long it takes to read 512MiB sequentially.
func BenchmarkRead512MiB(b *testing.B) {
	testDir := build.TempDir("persist", b.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(1 << 29)

	// Make the file.
	filename := filepath.Join(testDir, "512MiB.file")
	f, err := os.Create(filename)
	if err != nil {
		b.Fatal(err)
	}

	// 2^7 writes of 4MiB.
	for i := 0; i < 1<<7; i++ {
		// Get the entropy separate from the timer.
		b.StopTimer()
		data := fastrand.Bytes(1 << 22)
		b.StartTimer()
		_, err = f.Write(data)
		if err != nil {
			b.Fatal(err)
		}
		// Sync after every write.
		err = f.Sync()
		if err != nil {
			b.Fatal(err)
		}
	}

	// Close the file.
	err = f.Close()
	if err != nil {
		b.Fatal(err)
	}

	// Check the sequential read speed.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Open the file.
		f, err := os.Open(filename)
		if err != nil {
			b.Fatal(err)
		}

		// Read the file 4 MiB at a time.
		for i := 0; i < 1<<6; i++ {
			data := make([]byte, 1<<22)
			_, err = f.Read(data)
			if err != nil {
				b.Fatal(err)
			}
		}

		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	err = os.Remove(filename)
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkRead512MiBRand checks how long it takes to read 512MiB randomly.
func BenchmarkRead512MiBRand(b *testing.B) {
	testDir := build.TempDir("persist", b.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(1 << 29)

	// Make the file.
	filename := filepath.Join(testDir, "512MiB.file")
	f, err := os.Create(filename)
	if err != nil {
		b.Fatal(err)
	}

	// 2^7 writes of 4MiB.
	for i := 0; i < 1<<7; i++ {
		// Get the entropy separate from the timer.
		b.StopTimer()
		data := fastrand.Bytes(1 << 22)
		b.StartTimer()
		_, err = f.Write(data)
		if err != nil {
			b.Fatal(err)
		}
		// Sync after every write.
		err = f.Sync()
		if err != nil {
			b.Fatal(err)
		}
	}

	// Close the file.
	err = f.Close()
	if err != nil {
		b.Fatal(err)
	}

	// Check the sequential read speed.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Open the file.
		f, err := os.Open(filename)
		if err != nil {
			b.Fatal(err)
		}

		// Read the file 4 MiB at a time.
		for i := 0; i < 1<<6; i++ {
			offset := int64(fastrand.Intn(1 << 6))
			offset *= 1 << 22
			data := make([]byte, 1<<22)
			_, err = f.ReadAt(data, offset)
			if err != nil {
				b.Fatal(err)
			}
		}

		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	err = os.Remove(filename)
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkTruncate512MiB checks how long it takes to truncate a 512 MiB file.
func BenchmarkTruncate512MiB(b *testing.B) {
	testDir := build.TempDir("persist", b.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(1 << 29)
	filename := filepath.Join(testDir, "512MiB.file")
	// Check the truncate speed.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make the file separate from the timer.
		b.StopTimer()
		f, err := os.Create(filename)
		if err != nil {
			b.Fatal(err)
		}

		// 2^7 writes of 4MiB.
		for i := 0; i < 1<<7; i++ {
			// Get the entropy separate from the timer.
			b.StopTimer()
			data := fastrand.Bytes(1 << 22)
			b.StartTimer()
			_, err = f.Write(data)
			if err != nil {
				b.Fatal(err)
			}
		}
		// Sync after writing.
		err = f.Sync()
		if err != nil {
			b.Fatal(err)
		}

		// Close the file.
		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		// Open the file.
		f, err = os.OpenFile(filename, os.O_RDWR, 0600)
		if err != nil {
			b.Fatal(err)
		}

		// Truncate the file.
		err = f.Truncate(0)
		if err != nil {
			b.Fatal(err)
		}

		// Sync.
		err = f.Sync()
		if err != nil {
			b.Fatal(err)
		}

		// Close.
		err = f.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	err = os.Remove(filename)
	if err != nil {
		b.Fatal(err)
	}
}
