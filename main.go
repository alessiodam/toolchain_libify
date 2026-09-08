package main

import (
	"fmt"
	"os"

	"github.com/alessiodam/toolchain_libify/cmd"
)

var version = "dev"

func main() {
	if err := cmd.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, "libify:", err)
		os.Exit(1)
	}
}
