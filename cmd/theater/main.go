package main

import (
	"os"

	"github.com/alex-poliushkin/theater/internal/theatercli"
)

func main() {
	os.Exit(theatercli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
