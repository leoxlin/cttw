package main

import (
	"fmt"
	"os"

	"github.com/llin/cttw/internal/cli"
	"github.com/llin/cttw/internal/daemon"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--daemon" {
		if err := daemon.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := cli.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "cttw error: %v\n", err)
		os.Exit(1)
	}
}
