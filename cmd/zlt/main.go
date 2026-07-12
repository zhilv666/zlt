package main

import (
	"log"
	"os"

	// Embed the IANA timezone database: Windows has no system tzdata, and
	// schedule timezones (time.LoadLocation) must work on end-user machines
	// without a Go toolchain installed.
	_ "time/tzdata"

	"zhulingtai/internal/app"
)

func main() {
	if err := app.Execute(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
