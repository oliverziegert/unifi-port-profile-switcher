package main

import (
	"os"

	"github.com/oliverziegert/unifi-port-profile-switcher/cmd"
)

func main() {
	os.Exit(cmd.Run(os.Args[1:], cmd.IO{Stdout: os.Stdout, Stderr: os.Stderr}))
}
