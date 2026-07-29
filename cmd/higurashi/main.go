package main

import (
	"context"
	"os"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
)

var version = "dev"

func main() {
	workingDirectory, err := os.Getwd()
	if err != nil {
		workingDirectory = "."
	}

	os.Exit(cli.Run(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		cli.Options{
			WorkingDirectory: workingDirectory,
			Version:          version,
		},
	))
}
