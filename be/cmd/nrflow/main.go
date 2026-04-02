package main

import (
	"os"

	"be/internal/cli"
)

func main() {
	cli.RegisterCLICommands()
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
