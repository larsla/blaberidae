package main

import (
	"bufio"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type CockroachStats struct {
	grapher *Grapher
	stop    bool
	inserts int
	selects int
	updates int
}

func NewCockroachStats(grapher *Grapher) *CockroachStats {
	c := &CockroachStats{
		grapher: grapher,
	}

	c.inserts, c.selects, c.updates, _ = c.fetch()

	go c.run()

	return c
}

func (c *CockroachStats) Stop() {
	c.stop = true
}

func (c *CockroachStats) run() {
	for {
		if c.stop {
			return
		}

		time.Sleep(time.Second * 1)

		t := time.Now()

		newInserts, newSelects, newUpdates, err := c.fetch()
		if err != nil {
			c.grapher.Save(Stat{
				Time:  t,
				Name:  "COCKROACH",
				Label: "ERROR",
				Value: 10,
			})
			continue
		}

		c.grapher.Save(Stat{
			Time:  t,
			Name:  "COCKROACH",
			Label: "INSERT",
			Value: float64(newInserts - c.inserts),
		})
		c.grapher.Save(Stat{
			Time:  t,
			Name:  "COCKROACH",
			Label: "SELECT",
			Value: float64(newSelects - c.selects),
		})
		c.grapher.Save(Stat{
			Time:  t,
			Name:  "COCKROACH",
			Label: "UPDATE",
			Value: float64(newUpdates - c.updates),
		})

		c.inserts = newInserts
		c.selects = newSelects
		c.updates = newUpdates
	}
}

func (c *CockroachStats) fetch() (inserts int, selects int, updates int, err error) {
	req, err := http.Get("http://localhost:8080/_status/vars")
	if err != nil {
		log.Println("ERROR getting metrics from cockroachdb:", err)
		return
	}
	defer req.Body.Close()

	reader := bufio.NewScanner(req.Body)
	for reader.Scan() {
		parts := strings.Split(reader.Text(), " ")
		if len(parts) > 1 {
			if parts[0] == "sql_insert_count" {
				inserts, err = strconv.Atoi(parts[1])
			}
			if parts[0] == "sql_select_count" {
				selects, err = strconv.Atoi(parts[1])
			}
			if parts[0] == "sql_update_count" {
				updates, err = strconv.Atoi(parts[1])
			}
			if err != nil {
				return
			}
		}
	}

	return
}
