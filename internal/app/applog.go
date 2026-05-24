package app

import (
	"log"
	"os"
	"path/filepath"
	"sync"
)

var appLogOnce sync.Once

func initAppLogger() {
	appLogOnce.Do(func() {
		if err := os.MkdirAll("data", 0o755); err != nil {
			return
		}
		path := filepath.Join("data", "app.log")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		log.SetOutput(file)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	})
}
