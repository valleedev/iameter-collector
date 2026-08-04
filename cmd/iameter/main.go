// Command iameter is the IA METER Collector CLI.
package main

import (
	"os"

	"github.com/valleedev/iameter-collector/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
