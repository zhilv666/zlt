package app

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func runHeadless(rt *Runtime) error {
	log.Printf("dashboard available at %s", rt.Address())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	<-signals
	return rt.Shutdown(context.Background())
}
