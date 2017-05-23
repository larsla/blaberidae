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

	db1, err := Start("db1", binaryName, []string{"start", "--insecure", "--store=node1", "--http-port=8083"})
	if err != nil {
		log.Println(err)
		return
	}
	defer db1.Stop()
	defer os.RemoveAll("./node1")

	time.Sleep(time.Second * 2)

	db2, err := Start("db2", binaryName, []string{"start", "--insecure", "--store=node2", "--port=26258", "--http-port=8081", "--join=localhost:26257"})
	if err != nil {
		log.Println(err)
		return
	}
	defer db2.Stop()
	defer os.RemoveAll("./node2")

	db3, err := Start("db3", binaryName, []string{"start", "--insecure", "--store=node3", "--port=26259", "--http-port=8082", "--join=localhost:26257"})
	if err != nil {
		log.Println(err)
		return
	}
	defer db3.Stop()
	defer os.RemoveAll("./node3")

	nodes := []*Proc{db1, db2, db3}

	createDB, err := Start("createDB", binaryName, []string{"sql", "--insecure", "-e", "CREATE DATABASE test;"})
	if err != nil {
		log.Println(err)
		return
	}
	time.Sleep(time.Second * 2)
	if createDB.Process != nil && createDB.Running {
		createDB.Process.Wait()
	}

	ports := []string{"26257", "26258", "26259"}

	grapher := NewGrapher()
	defer grapher.Render(fmt.Sprintf("%s.png", time.Now().String()))
	defer grapher.Stop()

	stats := NewSysStats(grapher)
	defer stats.Stop()

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
						Value: 100,
					})
					return
				}

				err = bench.Insert(10)
				if err != nil {
					log.Println(mynum, "Insert", err)
					grapher.Save(Stat{
						Time:  time.Now(),
						Name:  "ERROR",
						Value: 100,
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

	threads := len(dones)
	for {
		if threads < 1 {
			fmt.Println("Done with threads")
			return
		}
		fmt.Printf("Waiting for %v threads\n", threads)
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
