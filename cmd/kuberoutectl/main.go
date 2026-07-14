// Command kuberoutectl is the CLI entrypoint. It is deliberately thin: build
// nothing, decide nothing — just hand off to the cli package and translate a
// returned error into a non-zero exit code.
package main

import (
	"fmt"
	"os"

	"github.com/ymedlop/kuberoutectl/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "kuberoutectl:", err)
		os.Exit(1)
	}
}
