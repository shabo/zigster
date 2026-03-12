package main

import (
	"os"

	"github.com/luki/sensors/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}
