// Package main is the entrypoint for the unifi-port-profile-switcher CLI.
package main

import (
	"fmt"
	"os"

	"github.com/oliverziegert/unifi-port-profile-switcher/cmd"
)

// version is overridden at release-build time via `-ldflags "-X main.version=<tag>"`.
var version = "dev"

func main() {
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-version" {
			_, _ = fmt.Fprintln(os.Stdout, version)
			os.Exit(0)
		}
	}
	os.Exit(cmd.Run(os.Args[1:], cmd.IO{Stdout: os.Stdout, Stderr: os.Stderr}))
}
