package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alessiodam/toolchain_libify/internal/ceenv"
	"github.com/spf13/cobra"
)

var (
	flagSelfBinary string
	flagSelfFasmg  string
	flagSelfVendor string
	flagSelfRemove bool
)

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func installFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := copyFile(source, target); err != nil {
		return err
	}
	if err := os.Chmod(target, 0o755); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "[install]", target)
	return nil
}

func removeFile(target string) {
	if err := os.RemoveAll(target); err == nil {
		fmt.Fprintln(os.Stderr, "[remove]", target)
	}
}

var selfInstallCmd = &cobra.Command{
	Use:   "selfinstall",
	Short: "Install or remove ce-libify inside a CE toolchain",
	Long: "Copies the ce-libify binary, the fasmg assembler and the fasmg macro\n" +
		"packages into the CE toolchain, so projects can call ce-libify without\n" +
		"knowing where any of it lives.",
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, err := ceenv.Prefix(flagCEdev)
		if err != nil {
			return err
		}

		binDir := filepath.Join(prefix, "bin")
		vendorDir := filepath.Join(prefix, "meta", "libify")
		binary := filepath.Join(binDir, "ce-libify"+exeSuffix())
		fasmg := filepath.Join(binDir, "fasmg"+exeSuffix())

		if flagSelfRemove {
			removeFile(binary)
			removeFile(fasmg)
			removeFile(vendorDir)
			return nil
		}

		if flagSelfBinary == "" || flagSelfFasmg == "" || flagSelfVendor == "" {
			return fmt.Errorf("--binary, --fasmg-source and --vendor are required")
		}

		if err := installFile(flagSelfBinary, binary); err != nil {
			return err
		}
		if err := installFile(flagSelfFasmg, fasmg); err != nil {
			return err
		}

		entries, err := os.ReadDir(flagSelfVendor)
		if err != nil {
			return err
		}

		installed := 0
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".inc") && !strings.HasSuffix(name, ".alm") {
				continue
			}
			if err := installFile(filepath.Join(flagSelfVendor, name), filepath.Join(vendorDir, name)); err != nil {
				return err
			}
			installed++
		}

		if installed == 0 {
			return fmt.Errorf("no macro packages found in %s, run get_fasmg.py first", flagSelfVendor)
		}

		fmt.Fprintf(os.Stderr, "[install] ce-libify is ready, %d macro packages in %s\n", installed, vendorDir)
		return nil
	},
}

func init() {
	selfInstallCmd.Flags().StringVar(&flagSelfBinary, "binary", "", "built ce-libify binary to install")
	selfInstallCmd.Flags().StringVar(&flagSelfFasmg, "fasmg-source", "", "fasmg binary to install")
	selfInstallCmd.Flags().StringVar(&flagSelfVendor, "vendor", "", "directory holding the fasmg macro packages")
	selfInstallCmd.Flags().BoolVar(&flagSelfRemove, "remove", false, "remove a previous installation")
	rootCmd.AddCommand(selfInstallCmd)
}
