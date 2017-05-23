package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"database/sql"
	"strings"

	_ "github.com/lib/pq"
)

type SQLTest struct {
	db      *sql.DB
	name    string
	grapher *Grapher
	ports   []string
}

func NewTest(name string, ports []string, grapher *Grapher) (*SQLTest, error) {
	s := &SQLTest{
		ports:   ports,
		name:    name,
		grapher: grapher,
	}
	s.connect()

	_, err := s.db.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id SERIAL PRIMARY KEY, value INT)", name))
	return s, err
}

func (s *SQLTest) connect() (err error) {
	s.db, err = sql.Open("postgres", fmt.Sprintf("host=localhost user=root dbname=test sslmode=disable port=%s", s.ports[rand.Intn(len(s.ports))]))

	return
}

func (s *SQLTest) reconnectIfDisconnected(err error) bool {
	if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connection reset") {
		err = s.connect()
		if err == nil {
			log.Println("Reconnected")
		}
		return true
	}

	return false
}

func (s *SQLTest) Create(num int) ([]int, error) {
	nums := make([]int, num)
	for i := range nums {
		nums[i] = 0
		if _, err := s.db.Exec(fmt.Sprintf("INSERT INTO %s (id, value) VALUES ($1, $2)", s.name), i+1, nums[i]); err != nil {
			return nil, err
		}
	}

	return nums, nil
}

func (s *SQLTest) Insert(num int) error {
	inserts := make([]time.Duration, 0)
	for i := 0; i < num; i++ {
		insertStart := time.Now()
		if _, err := s.db.Exec(fmt.Sprintf("INSERT INTO %s (value) VALUES (0)", s.name)); err != nil {
			return err
		}
		inserts = append(inserts, time.Since(insertStart))
	}

	var insertTot time.Duration
	for _, ins := range inserts {
		insertTot = insertTot + ins
	}
	insertTot = insertTot / time.Duration(len(inserts))
	s.grapher.Save(Stat{
		Time:  time.Now(),
		Name:  "INSERT",
		Value: insertTot.Seconds() * 1000,
	})
	return nil
}

func (s *SQLTest) Increment(nums []int) ([]int, error) {
	selects := make([]time.Duration, 0)
	updates := make([]time.Duration, 0)
	for i := range nums {
		selectStart := time.Now()
		rows, err := s.db.Query(fmt.Sprintf("SELECT id, value FROM %s WHERE id = $1", s.name), i+1)
		if err != nil {
			if !s.reconnectIfDisconnected(err) {
				return nil, err
			}
			return s.Increment(nums)
		}
		defer rows.Close()
		selects = append(selects, time.Since(selectStart))

		rows.Next()

		var id, value int
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}

		if value != nums[i] {
			return nil, fmt.Errorf("Expected value to be %v, but found %v", nums[i], value)
		}

		value = value + 1
		nums[i] = value
		updateStart := time.Now()
		if _, err := s.db.Exec(fmt.Sprintf("UPDATE %s set value = $1 WHERE id = $2", s.name), value, i+1); err != nil {
			if !s.reconnectIfDisconnected(err) {
				return nil, err
			}
			return s.Increment(nums)
		}
		updates = append(updates, time.Since(updateStart))
	}

	var selectTot time.Duration
	for _, sel := range selects {
		selectTot = selectTot + sel
	}
	selectTot = selectTot / time.Duration(len(selects))

	var updateTot time.Duration
	for _, upd := range updates {
		updateTot = updateTot + upd
	}
	updateTot = updateTot / time.Duration(len(updates))

	s.grapher.Save(Stat{
		Time:  time.Now(),
		Name:  "SELECT",
		Value: selectTot.Seconds() * 1000,
	})

	s.grapher.Save(Stat{
		Time:  time.Now(),
		Name:  "UPDATE",
		Value: updateTot.Seconds() * 1000,
	})

	return nums, nil
}
