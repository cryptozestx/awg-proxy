package main

import (
	"fmt"
	"log"
	"os"

	"awg-proxy/internal/app"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "\x1b[1;31mError: %v\x1b[0m\n", err)
		os.Exit(1)
	}
}
