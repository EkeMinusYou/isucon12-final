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

	if *cpuprofile != "" {
		cpu, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}

		fcpu, err := os.Create(*fcpuprofile)
		if err != nil {
			log.Fatal(err)
		}

		err = pprof.StartCPUProfile(cpu)
		if err != nil {
			log.Fatal(err)
		}

		stop := fgprof.Start(fcpu, fgprof.FormatPprof)
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
			<-sig
			pprof.StopCPUProfile()
			cpu.Close()
			stop()
			fcpu.Close()
			os.Exit(0)
		}()
	}

	Run()
}
