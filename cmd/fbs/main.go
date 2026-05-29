package main

import (
	"fmt"
	"os"

	"framework-battery-saver/internal/ui"
)

func main() {
	if err := ui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fbs:", err)
		os.Exit(1)
	}
}
