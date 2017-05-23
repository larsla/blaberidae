package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"flag"

	_ "github.com/lib/pq"
)

func main() {

	numThreads := flag.Int("threads", 10, "Number of threads to run")
	numIterations := flag.Int("iterations", 10, "Number of iterations")
	numNodes := flag.Int("nodes", 3, "Number of nodes")
	killNodes := flag.Bool("kill_nodes", false, "Randomly kill nodes during run")
	flag.Parse()

	binaryName := fmt.Sprintf("./cockroach-latest.%s-%s/cockroach", runtime.GOOS, runtime.GOARCH)
	_, err := os.Stat(binaryName)
	if err != nil {
		err := downloadFile("cockroach.tar.gz", fmt.Sprintf("https://binaries.cockroachdb.com/cockroach-latest.%s-%s.tgz", runtime.GOOS, runtime.GOARCH))
		if err != nil {
			log.Println(err)
			return
		}

		err = unpackTarGz("cockroach.tar.gz", ".")
		if err != nil {
			log.Println(err)
			return
		}
	}

	db1, err := Start("db1", binaryName, []string{"start", "--insecure", "--store=node1", "--http-port=8080"})
	if err != nil {
		log.Println(err)
		return
	}
	defer db1.Stop()
	defer os.RemoveAll("./node1")

	time.Sleep(time.Second * 2)

	ports := []string{"26257"}
	nodes := []*Proc{}
	for i := 2; i < *numNodes+1; i++ {
		port := 26256 + i
		httpPort := 8079 + i
		dbX, err := Start(fmt.Sprintf("db%v", i), binaryName, []string{"start", "--insecure", fmt.Sprintf("--store=node%v", i), fmt.Sprintf("--port=%v", port), fmt.Sprintf("--http-port=%v", httpPort), "--join=localhost:26257"})
		if err != nil {
			log.Println(err)
			return
		}
		defer dbX.Stop()
		defer os.RemoveAll(fmt.Sprintf("./node%v", i))

		ports = append(ports, fmt.Sprintf("%v", port))
		nodes = append(nodes, dbX)
	}

	createDB, err := Start("createDB", binaryName, []string{"sql", "--insecure", "-e", "CREATE DATABASE test;"})
	if err != nil {
		log.Println(err)
		return
	}
	time.Sleep(time.Second * 2)
	if createDB.Process != nil && createDB.Running {
		createDB.Process.Wait()
	}

	grapher := NewGrapher()
	defer grapher.Render(fmt.Sprintf("%s.png", time.Now().String()))
	defer grapher.Stop()

	stats := NewSysStats(grapher)
	defer stats.Stop()

	cockroachStats := NewCockroachStats(grapher)
	defer cockroachStats.Stop()

	stop := false
	start := false
	dones := make([]chan bool, 0)
	for i := 0; i < *numThreads; i++ {
		done := make(chan bool)
		dones = append(dones, done)

		grapher.Save(Stat{
			Time:  time.Now(),
			Name:  "threads",
			Value: float64(i),
		})

		go func(mynum int) {
			defer func() {
				done <- true
			}()

			log.Println(mynum, "Creating table")
			bench, err := NewTest(fmt.Sprintf("test%v", mynum), ports, grapher)
			if err != nil {
				log.Println(mynum, "NewTest", err)
				return
			}
			log.Println(mynum, "Preparing table")
			nums, err := bench.Create(10)
			if err != nil {
				log.Println(mynum, "Insert", err)
				return
			}

			for {
				if start {
					break
				}
				if stop {
					return
				}
				time.Sleep(time.Millisecond * 100)
			}
			for j := 0; j < *numIterations; j++ {
				if stop {
					return
				}
				_, err = bench.Increment(nums)
				if err != nil {
					log.Println(mynum, "Increment", err)
					grapher.Save(Stat{
						Time:  time.Now(),
						Name:  "ERROR",
						Value: 10,
					})
					grapher.Save(Stat{
						Time:  time.Now().Add(time.Millisecond * 10),
						Name:  "ERROR",
						Value: 0,
					})
					return
				}

				err = bench.Insert(10)
				if err != nil {
					log.Println(mynum, "Insert", err)
					grapher.Save(Stat{
						Time:  time.Now(),
						Name:  "ERROR",
						Value: 10,
					})
					grapher.Save(Stat{
						Time:  time.Now().Add(time.Millisecond * 10),
						Name:  "ERROR",
						Value: 0,
					})
					return
				}
			}
		}(i)
		time.Sleep(time.Millisecond * 500)
	}

	start = true

	// catch ctrl+c
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	restartLock := false
	restartChan := make(chan bool)
	if *killNodes {
		go func() {
			for {
				if stop {
					return
				}
				time.Sleep(time.Second * time.Duration(rand.Intn(60)))
				if !restartLock {
					restartChan <- true
				}
			}
		}()
	}

	lastPrinted := time.Now()
	threads := len(dones)
	for {
		if threads < 1 {
			fmt.Println("Done with threads")
			return
		}
		if time.Since(lastPrinted) > time.Second*2 {
			fmt.Printf("Waiting for %v threads\n", threads)
			lastPrinted = time.Now()
		}
		for i, done := range dones {
			select {
			case <-done:
				fmt.Printf("Got done from thread %v\n", i)
				threads = threads - 1
				grapher.Save(Stat{
					Time:  time.Now(),
					Name:  "threads",
					Value: float64(threads),
				})
			case <-c:
				fmt.Println("Got CTRL+C")
				return
			case <-restartChan:
				if !restartLock {
					restartLock = true
					node := nodes[rand.Intn(len(nodes))]
					log.Println("Restarting " + node.Name)
					grapher.Event(Event{
						Time: time.Now(),
						Name: "Restarted " + node.Name,
					})
					node.Restart()
					go func() {
						time.Sleep(time.Second * 2)
						restartLock = false
					}()
				}
			default:
			}
		}
		time.Sleep(time.Millisecond * 100)
	}
}
