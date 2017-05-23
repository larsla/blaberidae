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
	Label string
	Value float64
}

type Event struct {
	Time time.Time
	Name string
}

type Grapher struct {
	Input      chan Stat
	series     map[string]map[string]*chart.TimeSeries
	stop       chan bool
	startTime  time.Time
	stopTime   time.Time
	events     []Event
	eventInput chan Event
}

func NewGrapher() *Grapher {
	g := &Grapher{
		Input:      make(chan Stat, 10000),
		series:     make(map[string]map[string]*chart.TimeSeries),
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
			label := stat.Label
			if label == "" {
				label = stat.Name
			}
			if !ok {
				g.series[stat.Name] = make(map[string]*chart.TimeSeries)
			}
			_, ok = g.series[stat.Name][label]
			if !ok {
				g.series[stat.Name][label] = &chart.TimeSeries{
					Name:    label,
					XValues: make([]time.Time, 0),
					YValues: make([]float64, 0),
				}
			}
			g.series[stat.Name][label].XValues = append(g.series[stat.Name][label].XValues, stat.Time)
			g.series[stat.Name][label].YValues = append(g.series[stat.Name][label].YValues, stat.Value)
		case event := <-g.eventInput:
			g.events = append(g.events, event)
		}
	}
}

func (g *Grapher) Stop() {
	g.stop <- true
}

func (g *Grapher) Render(name string) error {

	log.Println("Starting to render graphs")

	g.stopTime = time.Now()

	annotations := make([]chart.Value2, 0)
	for _, annotation := range g.events {
		annotations = append(annotations, chart.Value2{
			XValue: util.Time.ToFloat64(annotation.Time),
			YValue: 80,
			Label:  annotation.Name,
		})
	}

	gcStats := debug.GCStats{}
	for _, gc := range gcStats.PauseEnd {
		annotations = append(annotations, chart.Value2{
			XValue: util.Time.ToFloat64(gc),
			YValue: 90,
			Label:  "GC",
		})
	}

	for seriesName := range g.series {
		log.Printf("Preparing graph %s", seriesName)
		series := make([]chart.Series, 0)
		series = append(series, chart.AnnotationSeries{
			Annotations: annotations,
		})
		max := float64(100)
		for label, l := range g.series[seriesName] {
			log.Printf("Processing label %s", label)
			for _, v := range l.YValues {
				if v > max {
					max = v
				}
			}
			series = append(series, l)
		}
		err := g.render(fmt.Sprintf("%s-%s", seriesName, name), series, max)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Grapher) render(name string, series []chart.Series, max float64) error {
	log.Printf("Rendering file %s", name)
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
				Max: max + (max / 5),
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
