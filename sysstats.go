package main

import (
	"log"
	"time"

	"github.com/c9s/goprocinfo/linux"
)

type SysStats struct {
	grapher *Grapher
	stop    bool
	idle    float64
	total   float64
	iowait  float64
}

func NewSysStats(grapher *Grapher) *SysStats {
	s := &SysStats{
		grapher: grapher,
		stop:    false,
	}

	s.idle, s.total, s.iowait = s.readStat()

	go s.run()

	return s
}

func (s *SysStats) Stop() {
	s.stop = true
}

func (s *SysStats) run() {
	for {
		if s.stop {
			return
		}

		time.Sleep(time.Second * 1)

		newIdle, newTotal, newIOWait := s.readStat()

		percentage := ((newTotal - s.total) - (newIdle - s.idle)) / (newTotal - s.total) * 100
		// ioWait := ((newTotal - s.total) - (newIdle - s.idle) - (s.iowait - newIOWait)) / (newTotal - s.total) * 100
		s.grapher.Save(Stat{
			Time:  time.Now(),
			Name:  "SYSTEM",
			Label: "CPU",
			Value: float64(percentage),
		})
		// s.grapher.Save(Stat{
		// 	Time:  time.Now(),
		// 	Name:  "SYSTEM",
		// 	Label: "IOWait",
		// 	Value: float64(ioWait),
		// })

		s.total = newTotal
		s.idle = newIdle
		s.iowait = newIOWait
	}
}

func (s *SysStats) readStat() (float64, float64, float64) {
	stat, err := linux.ReadStat("/proc/stat")
	if err != nil {
		log.Fatal("stat read fail")
	}

	info := stat.CPUStatAll
	idle := info.Idle + info.IOWait
	nonIdle := info.User + info.System + info.IRQ + info.SoftIRQ + info.Steal
	total := idle + nonIdle

	return float64(idle), float64(total), float64(info.IOWait)
}
