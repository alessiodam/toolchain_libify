package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove the generated object and output directories",
	Long: "Removing the build directories from Go rather than the shell keeps the\n" +
		"makefile working the same whether make picked cmd.exe or a POSIX shell.",
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, dir := range []string{flagObjDir, flagBinDir} {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "[clean]", dir)
		}
		return nil
	},
}

func init() {
	cleanCmd.Flags().StringVar(&flagObjDir, "obj", "obj", "intermediate output directory")
	cleanCmd.Flags().StringVar(&flagBinDir, "bin", "bin", "output directory for the appvar and stub")
	rootCmd.AddCommand(cleanCmd)
}
