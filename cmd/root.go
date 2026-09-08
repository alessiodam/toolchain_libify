package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var (
	flagCEdev   string
	flagFasmg   string
	flagVerbose bool
)

var rootCmd = &cobra.Command{
	Use:   "ce-libify",
	Short: "Build a LibLoad library for the TI-84 Plus CE out of C sources",
	Long: "ce-libify compiles a directory of C sources into a single ez80 assembly\n" +
		"module, rewrites it into something fasmg can assemble, and emits the\n" +
		"LibLoad appvar together with the .lib stub that applications link against.",
	SilenceUsage:  true,
	SilenceErrors: false,
}

func Execute(version string) error {
	rootCmd.Version = version
	return rootCmd.Execute()
}

func requireName() error {
	if flagName == "" {
		return errors.New("--name is required, it is the LibLoad library name")
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagCEdev, "cedev", "", "CE toolchain prefix (default: cedev-config --prefix)")
	rootCmd.PersistentFlags().StringVar(&flagFasmg, "fasmg", "", "path to the fasmg binary")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "echo every command that is run")
}
