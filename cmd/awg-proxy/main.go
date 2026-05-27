package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"awg-proxy/internal/app"
)

var version = "1.0.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	app.Version = version
	if err := app.Run(os.Args); err != nil {
		var usageErr app.UsageError
		if errors.As(err, &usageErr) {
			fmt.Fprintf(os.Stderr, "\x1b[1;31mError: %v\x1b[0m\n", err)
			if usageErr.BlankLineBeforeUsage {
				fmt.Fprintln(os.Stderr)
			}
			app.PrintUsage(os.Stderr)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "\x1b[1;31mError: %v\x1b[0m\n", err)
		os.Exit(1)
	}
}
