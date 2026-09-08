package cmd

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	flagOutDir  string
	flagRelease string
)

func addToArchive(archive *zip.Writer, source string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.Base(source)
	header.Method = zip.Deflate

	out, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	return err
}

var packageCmd = &cobra.Command{
	Use:   "package",
	Short: "Stage the release archive",
	Long: "Collects the three files a release consists of - the appvar, the link\n" +
		"stub and the public header - into a zip. Internal headers are never\n" +
		"included.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireName(); err != nil {
			return err
		}

		files := []string{
			filepath.Join(flagBinDir, flagName+".8xv"),
			filepath.Join(flagBinDir, flagName+".lib"),
		}
		files = append(files, flagHeaders...)

		for _, file := range files {
			if _, err := os.Stat(file); err != nil {
				return fmt.Errorf("%s is missing, build first", file)
			}
		}

		if err := os.MkdirAll(flagOutDir, 0o755); err != nil {
			return err
		}

		name := flagName
		if flagRelease != "" {
			name += "-" + flagRelease
		}
		target := filepath.Join(flagOutDir, name+".zip")

		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()

		archive := zip.NewWriter(out)
		for _, file := range files {
			if err := addToArchive(archive, file); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "[package]", filepath.Base(file))
		}
		if err := archive.Close(); err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}

		for _, file := range files {
			if err := copyFile(file, filepath.Join(flagOutDir, filepath.Base(file))); err != nil {
				return err
			}
		}

		fmt.Fprintln(os.Stderr, "[package]", target)
		return nil
	},
}

func init() {
	packageCmd.Flags().StringVar(&flagName, "name", "", "LibLoad library name (required)")
	packageCmd.Flags().StringVar(&flagBinDir, "bin", "bin", "directory holding the built appvar and stub")
	packageCmd.Flags().StringSliceVar(&flagHeaders, "header", nil, "public header to include")
	packageCmd.Flags().StringVar(&flagOutDir, "out", "release", "directory to stage the release in")
	packageCmd.Flags().StringVar(&flagRelease, "release", "", "version label appended to the archive name")
	rootCmd.AddCommand(packageCmd)
}
