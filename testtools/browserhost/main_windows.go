//go:build windows

// browserhost is an integration-test fixture that behaves like a browser root
// process: it launches a separate renderer process which owns the audio stream.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func main() {
	renderer := flag.String("renderer", "", "path to the audio renderer fixture")
	frequency := flag.Int("frequency", 523, "renderer tone frequency in Hz")
	seconds := flag.Int("seconds", 3, "renderer lifetime in seconds")
	flag.Parse()
	if *renderer == "" {
		fmt.Fprintln(os.Stderr, "-renderer is required")
		os.Exit(2)
	}
	command := exec.Command(*renderer,
		fmt.Sprintf("-frequency=%d", *frequency),
		fmt.Sprintf("-seconds=%d", *seconds),
	)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
