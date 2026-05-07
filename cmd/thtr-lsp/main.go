package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alex-poliushkin/theater/internal/thtrlsp"
)

func main() {
	if err := thtrlsp.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "thtr-lsp: %v\n", err)
		os.Exit(1)
	}
}
