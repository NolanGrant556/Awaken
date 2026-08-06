package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "network proxy observer unavailable")
		os.Exit(1)
	}
}
