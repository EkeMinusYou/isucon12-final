package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/felixge/fgprof"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var fcpuprofile = flag.String("fcpuprofile", "", "write fgprof cpu profile to file")

func main() {
	flag.Parse()

	var cpuCloser func()
	var stopFGProf func() error

	if *cpuprofile != "" {
		cpu, closer := startCPUProfile(*cpuprofile)
		defer cpu.Close()
		cpuCloser = closer
	}

	if *fcpuprofile != "" {
		fcpu, stop := startFGProfile(*fcpuprofile)
		defer fcpu.Close()
		stopFGProf = stop
	}

	handleSignals(cpuCloser, stopFGProf)
	Run()

}

func startCPUProfile(cpuprofile string) (*os.File, func()) {
	cpu, err := os.Create(cpuprofile)
	if err != nil {
		log.Fatal(err)
	}

	err = pprof.StartCPUProfile(cpu)
	if err != nil {
		log.Fatal(err)
	}

	return cpu, pprof.StopCPUProfile
}

func startFGProfile(fcpuprofile string) (*os.File, func() error) {
	fcpu, err := os.Create(fcpuprofile)
	if err != nil {
		log.Fatal(err)
	}

	stop := fgprof.Start(fcpu, fgprof.FormatPprof)
	return fcpu, stop
}

func handleSignals(cpuCloser func(), stopFGProf func() error) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		<-sig
		if cpuCloser != nil {
			cpuCloser()
		}
		if stopFGProf != nil {
			stopFGProf()
		}
		os.Exit(0)
	}()
}
