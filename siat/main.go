package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type replace struct {
	c chan struct{}
}

// TODO: turn this into a benchmark in the storagemanager2
func main() {
	/*
		var r replace
		r.c = make(chan struct{})
		tt := make(chan struct{})
		aa := make(chan struct{})
		go func(c chan struct{}) {
			<-tt
			<-c
			fmt.Println("whoop")
			close(aa)
		}(r.c)

		close(r.c)
		r.c = make(chan struct{})
		close(tt)
		<-aa
	*/

	fmt.Println("bf scanning time!")

	field := make([]byte, 1<<20)
	field = append(field, 1)

	fmt.Println("starting scan")
	start := time.Now()
	for i := 0; i < 100; i++ {
		reduce := bytes.TrimLeft(field, string(byte(0)))
		if len(reduce) != 1 {
			panic("wrong")
		}
	}
	fmt.Println("done:", time.Since(start).Seconds())

	fmt.Println("Serial fsync test")
	testData := make([]byte, 1<<20)
	for i := range testData {
		testData[i] = byte(i)
	}
	start = time.Now()
	for i := 0; i < 150; i++ {
		filename := strconv.Itoa(i)
		file, err := os.Create("/home/david/siat/" + filename)
		if err != nil {
			panic(err)
		}
		file.Write(testData)
		file.Sync()
		file.Close()
	}
	fmt.Println("done:", time.Since(start).Seconds())

	fmt.Println("Sync1")
	start = time.Now()
	syscall.Sync()
	fmt.Println("Sync1 complete:", time.Since(start).Seconds())

	fmt.Println("Sync2")
	start = time.Now()
	syscall.Sync()
	fmt.Println("Sync2 complete:", time.Since(start).Seconds())

	fmt.Println("Parallel fsync test")
	start = time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			filename := strconv.Itoa(i)
			file, err := os.Create("/home/david/siat/" + filename)
			if err != nil {
				panic(err)
			}
			file.Write(testData)
			file.Sync()
			file.Close()
			wg.Done()
		}()
		wg.Wait()
	}
	fmt.Println("done:", time.Since(start).Seconds())

	fmt.Println("Serial trailing fsync test")
	start = time.Now()
	for i := 0; i < 150; i++ {
		filename := strconv.Itoa(i)
		file, err := os.Create("/home/david/siat/" + filename)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		file.Write(testData)
	}
	syscall.Sync()
	fmt.Println("done:", time.Since(start).Seconds())

	fmt.Println("Parallel trailing fsync test")
	start = time.Now()
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			filename := strconv.Itoa(i)
			file, err := os.Create("/home/david/siat/" + filename)
			if err != nil {
				panic(err)
			}
			file.Write(testData)
			defer file.Close()
			wg.Done()
		}()
		wg.Wait()
	}
	syscall.Sync()
	fmt.Println("done:", time.Since(start).Seconds())

	fmt.Println("Serial no fsync")
	start = time.Now()
	for i := 0; i < 150; i++ {
		filename := strconv.Itoa(i)
		file, err := os.Create("/home/david/siat/" + filename)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		file.Write(testData)
	}
	fmt.Println("done:", time.Since(start).Seconds())

	fmt.Println("Parallel no fsync")
	start = time.Now()
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			filename := strconv.Itoa(i)
			file, err := os.Create("/home/david/siat/" + filename)
			if err != nil {
				panic(err)
			}
			file.Write(testData)
			defer file.Close()
			wg.Done()
		}()
		wg.Wait()
	}
	fmt.Println("done:", time.Since(start).Seconds())
}
