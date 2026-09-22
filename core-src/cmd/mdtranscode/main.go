package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mdtranscode/mdtranscode/core-src/transcode"
)

type cliOptions struct {
	input  string
	output string
	format string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	options, help, err := parseArgs(args)
	if help {
		printUsage(stdout)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "mdtranscode: %v\n\n", err)
		printUsage(stderr)
		return 2
	}

	if err := convert(options); err != nil {
		fmt.Fprintf(stderr, "mdtranscode: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Created %s\n", options.output)
	return 0
}

func parseArgs(args []string) (cliOptions, bool, error) {
	options := cliOptions{format: "docx"}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			return options, true, nil
		case arg == "--to":
			if i+1 >= len(args) {
				return options, false, errors.New("--to requires a format")
			}
			i++
			options.format = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--to="):
			options.format = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--to=")))
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) {
				return options, false, fmt.Errorf("%s requires a path", arg)
			}
			i++
			options.output = args[i]
		case strings.HasPrefix(arg, "--output="):
			options.output = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-"):
			return options, false, fmt.Errorf("unknown option %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return options, false, errors.New("input Markdown file is required")
	}
	if len(positional) > 1 {
		return options, false, errors.New("only one input file is supported in this version")
	}

	options.input = positional[0]

	if options.format == "" {
		options.format = "docx"
	}
	if options.format != "docx" {
		return options, false, fmt.Errorf("unsupported output format %q; currently supported: docx", options.format)
	}

	if options.output == "" {
		options.output = defaultOutputPath(options.input, options.format)
	}

	return options, false, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "MDTranscode - turn Markdown into professional documents")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mdtranscode input.md --to docx")
	fmt.Fprintln(w, "  mdtranscode input.md --to docx --output report.docx")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --to FORMAT        Output format. Currently: docx")
	fmt.Fprintln(w, "  -o, --output PATH  Explicit output path")
	fmt.Fprintln(w, "  -h, --help         Show this help")
}

func defaultOutputPath(input, format string) string {
	return transcode.DefaultOutputPath(input, format)
}

func convert(options cliOptions) error {
	_, err := transcode.ConvertFile(transcode.FileOptions{
		InputPath:  options.input,
		OutputPath: options.output,
		Format:     options.format,
	})
	return err
}
