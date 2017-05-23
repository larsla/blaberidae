package main

import (
	"fmt"
	"log"
	"time"

	"os"

	"reflect"

	"runtime/debug"

	chart "github.com/wcharczuk/go-chart"
	util "github.com/wcharczuk/go-chart/util"
)

type Stat struct {
	Time  time.Time
	Name  string
	Value float64
}

type Event struct {
	Time time.Time
	Name string
}

type Grapher struct {
	Input      chan Stat
	series     map[string]*chart.TimeSeries
	stop       chan bool
	startTime  time.Time
	stopTime   time.Time
	events     []Event
	eventInput chan Event
}

func NewGrapher() *Grapher {
	g := &Grapher{
		Input:      make(chan Stat, 10000),
		series:     make(map[string]*chart.TimeSeries),
		stop:       make(chan bool),
		startTime:  time.Now(),
		eventInput: make(chan Event, 10000),
	}

	go g.run()

	return g
}

func (g *Grapher) Save(stat Stat) {
	g.Input <- stat
}

func (g *Grapher) Event(event Event) {
	g.eventInput <- event
}

func (g *Grapher) run() {
	for {
		select {
		case <-g.stop:
			return
		case stat := <-g.Input:
			_, ok := g.series[stat.Name]
			if !ok {
				g.series[stat.Name] = &chart.TimeSeries{
					Name:    stat.Name,
					XValues: make([]time.Time, 0),
					YValues: make([]float64, 0),
				}
			}
			g.series[stat.Name].XValues = append(g.series[stat.Name].XValues, stat.Time)
			g.series[stat.Name].YValues = append(g.series[stat.Name].YValues, stat.Value)
		case event := <-g.eventInput:
			g.events = append(g.events, event)
		}
	}
}

func (g *Grapher) Stop() {
	g.stop <- true
}

func (g *Grapher) Render(name string) error {

	g.stopTime = time.Now()

	annotations := make([]chart.Value2, 0)
	for _, annotation := range g.events {
		annotations = append(annotations, chart.Value2{
			XValue: util.Time.ToFloat64(annotation.Time),
			YValue: 400,
			Label:  annotation.Name,
		})
	}

	gcStats := debug.GCStats{}
	for _, gc := range gcStats.PauseEnd {
		annotations = append(annotations, chart.Value2{
			XValue: util.Time.ToFloat64(gc),
			YValue: 450,
			Label:  "GC",
		})
	}

	for _, s := range g.series {
		series := make([]chart.Series, 0)
		series = append(series, chart.AnnotationSeries{
			Annotations: annotations,
		})
		series = append(series, s)
		err := g.render(fmt.Sprintf("%s-%s", s.Name, name), series)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Grapher) render(name string, series []chart.Series) error {
	graph := chart.Chart{
		Width:  2560,
		Height: 1440,
		XAxis: chart.XAxis{
			Name:      "Time",
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
			ValueFormatter: func(v interface{}) string {
				if vf, isTime := v.(float64); isTime {
					return util.Time.FromFloat64(vf).Sub(g.startTime).String()
				}
				log.Println("Unknown value type:", reflect.TypeOf(v).String())
				return ""
			},
		},
		YAxis: chart.YAxis{
			Name:      "Value",
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
			ValueFormatter: func(v interface{}) string {
				if vf, isFloat := v.(float64); isFloat {
					return fmt.Sprintf("%0.2f", vf)
				}
				return ""
			},
			Range: &chart.ContinuousRange{
				Min: 0,
				Max: 500,
			},
		},
		Series: series,
		Background: chart.Style{
			Padding: chart.Box{
				Top:  20,
				Left: 20,
			},
		},
	}
	graph.Elements = []chart.Renderable{
		chart.Legend(&graph),
	}

	out, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}

	err = graph.Render(chart.PNG, out)
	if err != nil {
		return err
	}
	out.Close()

	return nil
}
