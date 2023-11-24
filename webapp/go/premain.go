package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"cloud.google.com/go/profiler"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"github.com/felixge/fgprof"
	_ "github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var fcpuprofile = flag.String("fcpuprofile", "", "write fgprof cpu profile to file")

func main() {
	ctx := context.Background()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID != "" {
		cfg := profiler.Config{
			Service:        "isuconquest",
			ServiceVersion: "1.0.0",
			ProjectID:      projectID,
		}

		if err := profiler.Start(cfg); err != nil {
			log.Fatalf("failed to start profiler: %v", err)
		}

		res, err := resource.New(ctx,
			resource.WithTelemetrySDK(),
		)
		if err != nil {
			log.Fatalf("resource.New: %v", err)
		}
		exporter, err := texporter.New(texporter.WithProjectID(projectID))
		if err != nil {
			log.Fatalf("texporter.New: %v", err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(res),
		)
		defer tp.Shutdown(ctx)
		otel.SetTracerProvider(tp)
		log.Printf("Tracing to project %s", projectID)
	} else {
		log.Printf("GOOGLE_CLOUD_PROJECT not set, not tracing")
	}

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
