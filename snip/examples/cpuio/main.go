// I/O-bound vs CPU-bound work, and what goroutines can and can't speed up
// (chapter 4.5).
//
//	go run ./examples/cpuio -kind io              # 8 waits: goroutines overlap them, ~8x faster
//	go run ./examples/cpuio -kind cpu             # 8 calculations: faster only up to your core count
//	go run ./examples/cpuio -kind cpu -procs 1    # one core allowed: goroutines don't help at all
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"runtime"
	"sync"
	"time"
)

func main() {
	kind := flag.String("kind", "io", `"io" (waiting) or "cpu" (calculating)`)
	jobs := flag.Int("jobs", 8, "how many jobs to run")
	procs := flag.Int("procs", 0, "GOMAXPROCS: how many cores Go may use at once (0 = all)")
	flag.Parse()

	if *procs > 0 {
		runtime.GOMAXPROCS(*procs) // limit how many goroutines can run on CPUs at the same moment
	}
	fmt.Printf("this machine has %d cores; Go may use %d at once\n\n", runtime.NumCPU(), runtime.GOMAXPROCS(0))

	var job func()
	switch *kind {
	case "io":
		job = waitForNetwork
	case "cpu":
		job = crunchNumbers
	default:
		fmt.Println(`-kind must be "io" or "cpu"`)
		return
	}

	// One after another: total time is the sum of every job.
	start := time.Now()
	for i := 0; i < *jobs; i++ {
		job()
	}
	seq := time.Since(start)
	fmt.Printf("%d %s jobs one after another: %v\n", *jobs, *kind, seq.Round(time.Millisecond))

	// All at once, one goroutine per job.
	start = time.Now()
	var wg sync.WaitGroup
	for i := 0; i < *jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job()
		}()
	}
	wg.Wait()
	conc := time.Since(start)
	fmt.Printf("%d %s jobs in goroutines:     %v  (speed-up: %.1fx)\n", *jobs, *kind, conc.Round(time.Millisecond), seq.Seconds()/conc.Seconds())
}

// waitForNetwork stands in for an I/O-bound job: a database query or an HTTP
// call. The program spends the whole time waiting, using no CPU at all.
func waitForNetwork() {
	time.Sleep(300 * time.Millisecond)
}

// crunchNumbers stands in for a CPU-bound job: resizing an image, hashing a
// password, compressing a file. The CPU is busy the whole time.
func crunchNumbers() {
	sum := sha256.Sum256([]byte("start"))
	for i := 0; i < 3_000_000; i++ {
		sum = sha256.Sum256(sum[:])
	}
	_ = sum
}
