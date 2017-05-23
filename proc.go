package main

import (
	"fmt"
	"os"
	"time"
)

type Proc struct {
	Name          string
	Binary        string
	PID           int
	Process       *os.Process
	Running       bool
	args          []string
	LatestRestart time.Time
}

func Start(name, p string, args []string) (*Proc, error) {
	proc := &Proc{
		Name:   name,
		Binary: p,
		args:   args,
	}
	var err error
	outFile, err := os.OpenFile(fmt.Sprintf("%s.log", name), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	wd, _ := os.Getwd()
	procAtr := &os.ProcAttr{
		Dir: wd,
		Env: os.Environ(),
		Files: []*os.File{
			os.Stdin,
			outFile,
			os.Stderr,
		},
	}
	args = append([]string{fmt.Sprintf("./%s", p)}, args...)
	proc.Process, err = os.StartProcess(p, args, procAtr)
	if err != nil {
		return nil, err
	}
	proc.Running = true
	go proc.check()

	proc.PID = proc.Process.Pid

	return proc, nil
}

func (p *Proc) check() {
	defer func() {
		if r := recover(); r != nil {
			p.Running = false
			p.Stop()
		}
	}()
	state, err := p.Process.Wait()
	if err != nil {
		fmt.Println("Error:", err)
	}
	if state.Exited() {
		fmt.Println(p.Name, "exited:", state.String())
	}
	p.Running = false

}

func (p *Proc) Stop() error {
	if p.Process == nil {
		return fmt.Errorf("No process")
	}
	return p.Process.Kill()
}

func (p *Proc) Restart() error {
	p.Stop()
	p2, err := Start(p.Name, p.Binary, p.args)
	p.Process = p2.Process
	p.PID = p2.PID
	go p.check()

	return err
}
