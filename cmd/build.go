package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alessiodam/toolchain_libify/internal/ceenv"
	"github.com/spf13/cobra"
)

var (
	flagName     string
	flagLibVer   int
	flagSrcDir   string
	flagHeadDir  string
	flagVendor   string
	flagObjDir   string
	flagBinDir   string
	flagExports  string
	flagDepends  []string
	flagOptimize string
)

func newPipeline() (*pipeline, error) {
	if err := requireName(); err != nil {
		return nil, err
	}

	env, err := ceenv.Detect(flagCEdev, flagFasmg, flagVerbose)
	if err != nil {
		return nil, err
	}

	vendor := flagVendor
	if vendor == "" {
		vendor = env.VendorDir()
	}

	return &pipeline{
		env:        env,
		name:       flagName,
		libVersion: flagLibVer,
		srcDir:     flagSrcDir,
		headerDir:  flagHeadDir,
		vendorDir:  vendor,
		objDir:     flagObjDir,
		binDir:     flagBinDir,
		exportFile: flagExports,
		dependency: flagDepends,
		optimize:   flagOptimize,
	}, nil
}

func writeImage(p *pipeline) (string, error) {
	image, deps, resolved, err := p.transform()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(p.objDir, 0o755); err != nil {
		return "", err
	}

	assembly := filepath.Join(p.objDir, p.name+".asm")
	if err := os.WriteFile(assembly, image, 0o644); err != nil {
		return "", err
	}

	for _, dep := range deps {
		fmt.Fprintf(os.Stderr, "[dep] %s\n", dep.Name)
	}
	fmt.Fprintf(os.Stderr, "[fix] %d os aliases, %d stubs\n", len(resolved.aliases), len(resolved.stubs))
	fmt.Fprintf(os.Stderr, "[asm] %s\n", assembly)

	return assembly, nil
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the LibLoad appvar and its .lib stub",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := newPipeline()
		if err != nil {
			return err
		}

		assembly, err := writeImage(p)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(p.binDir, 0o755); err != nil {
			return err
		}

		appvar := filepath.Join(p.binDir, p.name+".8xv")
		if err := p.env.Run(p.env.Fasmg, assembly, appvar); err != nil {
			return err
		}

		info, err := os.Stat(appvar)
		if err != nil {
			return err
		}

		stub := filepath.Join(p.binDir, p.name+".lib")
		if _, err := os.Stat(stub); err != nil {
			return fmt.Errorf("fasmg did not produce %s", stub)
		}

		fmt.Fprintf(os.Stderr, "[success] %s, %d bytes\n", appvar, info.Size())
		fmt.Fprintf(os.Stderr, "[success] %s\n", stub)
		return nil
	},
}

var emitCmd = &cobra.Command{
	Use:   "emit",
	Short: "Write the generated assembly without running fasmg",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := newPipeline()
		if err != nil {
			return err
		}
		_, err = writeImage(p)
		return err
	},
}

var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Report how every external symbol gets resolved",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := newPipeline()
		if err != nil {
			return err
		}

		_, deps, resolved, err := p.transform()
		if err != nil {
			return err
		}

		for _, dep := range deps {
			fmt.Printf("library %s (%d exports)\n", dep.Name, len(dep.Exports))
		}
		for name, target := range resolved.aliases {
			fmt.Printf("os      %s -> %s\n", name, target)
		}
		for _, name := range resolved.stubs {
			fmt.Printf("stub    %s\n", name)
		}
		return nil
	},
}

func addPipelineFlags(commands ...*cobra.Command) {
	for _, command := range commands {
		flags := command.Flags()
		flags.StringVar(&flagName, "name", "", "LibLoad library name (required)")
		flags.IntVar(&flagLibVer, "lib-version", 0, "LibLoad library version")
		flags.StringVar(&flagSrcDir, "src", "src", "directory holding the library C sources")
		flags.StringVar(&flagHeadDir, "headers", ".", "directory holding the public headers")
		flags.StringVar(&flagVendor, "vendor", "", "directory holding the fasmg macro packages (default: <cedev>/meta/libify)")
		flags.StringVar(&flagObjDir, "obj", "obj", "intermediate output directory")
		flags.StringVar(&flagBinDir, "bin", "bin", "output directory for the appvar and stub")
		flags.StringVar(&flagExports, "exports", "exports.txt", "ordered export list")
		flags.StringSliceVar(&flagDepends, "depend", nil, "LibLoad library this one depends on")
		flags.StringVar(&flagOptimize, "optimize", "-Oz", "optimisation level passed to ez80-clang")
	}
}

func init() {
	addPipelineFlags(buildCmd, emitCmd, depsCmd)
	rootCmd.AddCommand(buildCmd, emitCmd, depsCmd)
}
