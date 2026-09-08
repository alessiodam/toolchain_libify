package cmd

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var declaration = regexp.MustCompile(`(?m)^[A-Za-z_][\w \t*]*?\b([a-z][a-z0-9_]*)\s*\(`)

func readExports(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var names []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)

	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if seen[text] {
			return nil, fmt.Errorf("%s:%d: duplicate export %q", path, line, text)
		}
		seen[text] = true
		names = append(names, text)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}

	return names, nil
}

func hasAnyPrefix(name string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func declaredIn(headers []string, prefixes []string) ([]string, error) {
	var names []string
	seen := map[string]bool{}

	for _, header := range headers {
		body, err := os.ReadFile(header)
		if err != nil {
			return nil, err
		}
		for _, match := range declaration.FindAllStringSubmatch(string(body), -1) {
			name := match[1]
			if seen[name] || !hasAnyPrefix(name, prefixes) {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}

	sort.Strings(names)
	return names, nil
}

var exportsCmd = &cobra.Command{
	Use:   "exports",
	Short: "Check exports.txt against the public headers",
	Long: "LibLoad resolves imports by ordinal, so exports.txt is append only:\n" +
		"reordering or removing a line silently breaks every application already\n" +
		"built against the library. This command reports drift between the list\n" +
		"and the headers without touching either file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		exports, err := readExports(flagExportFile)
		if err != nil {
			return err
		}

		declared, err := declaredIn(flagHeaders, flagPrefixes)
		if err != nil {
			return err
		}

		listed := map[string]bool{}
		for _, name := range exports {
			listed[name] = true
		}
		known := map[string]bool{}
		for _, name := range declared {
			known[name] = true
		}

		var unexported, undeclared []string
		for _, name := range declared {
			if !listed[name] {
				unexported = append(unexported, name)
			}
		}
		for _, name := range exports {
			if !known[name] {
				undeclared = append(undeclared, name)
			}
		}

		fmt.Printf("%d exports, %d declarations\n", len(exports), len(declared))

		for _, name := range unexported {
			fmt.Printf("  declared but not exported: %s (append it to %s)\n", name, flagExportFile)
		}
		for _, name := range undeclared {
			fmt.Printf("  exported but not declared:  %s\n", name)
		}

		if len(unexported)+len(undeclared) > 0 {
			return fmt.Errorf("exports.txt and the headers disagree")
		}

		fmt.Println("exports.txt matches the headers")
		return nil
	},
}

var (
	flagExportFile string
	flagHeaders    []string
	flagPrefixes   []string
)

func init() {
	exportsCmd.Flags().StringVar(&flagExportFile, "exports", "exports.txt", "ordered export list")
	exportsCmd.Flags().StringSliceVar(&flagHeaders, "header", nil, "header to read declarations from")
	exportsCmd.Flags().StringSliceVar(&flagPrefixes, "prefix", nil, "only consider declarations with these prefixes")
	rootCmd.AddCommand(exportsCmd)
}
