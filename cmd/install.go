package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alessiodam/toolchain_libify/internal/ceenv"
	"github.com/spf13/cobra"
)

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Copy the .lib stub and public headers into the CE toolchain",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireName(); err != nil {
			return err
		}

		prefix, err := ceenv.Prefix(flagCEdev)
		if err != nil {
			return err
		}

		stub := filepath.Join(flagBinDir, flagName+".lib")
		target := filepath.Join(prefix, "lib", "libload", flagName+".lib")
		if err := copyFile(stub, target); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "[install]", target)

		for _, header := range flagHeaders {
			target := filepath.Join(prefix, "include", filepath.Base(header))
			if err := copyFile(header, target); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "[install]", target)
		}

		return nil
	},
}

func init() {
	installCmd.Flags().StringVar(&flagName, "name", "", "LibLoad library name (required)")
	installCmd.Flags().StringVar(&flagBinDir, "bin", "bin", "directory holding the built stub")
	installCmd.Flags().StringSliceVar(&flagHeaders, "header", nil, "public header to install")
	rootCmd.AddCommand(installCmd)
}
