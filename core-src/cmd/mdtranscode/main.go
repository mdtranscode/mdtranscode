package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mdtranscode/mdtranscode/core-src/renderer/docx"
	"github.com/mdtranscode/mdtranscode/core-src/renderer/pdf"
	"github.com/mdtranscode/mdtranscode/core-src/transcode"
	"strconv"
)

type cliOptions struct {
	input  string
	output string
	format string
	pdf    pdf.Options
	word   docx.Options
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
	options := cliOptions{format: "docx", pdf: pdf.Defaults()}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			return options, true, nil
		case arg == "--word-toc":
			options.word.TOC = true
		case arg == "--word-page-numbers":
			options.word.PageNumbers = true
		case arg == "--word-break-h1":
			options.word.BreakBeforeH1 = true
		case arg == "--word-landscape":
			options.word.Landscape = true
		case strings.HasPrefix(arg, "--word-"):
			flag, value, hasEquals := strings.Cut(arg, "=")
			if !hasEquals {
				if i+1 >= len(args) {
					return options, false, fmt.Errorf("%s requires a value", flag)
				}
				i++
				value = args[i]
			}
			if err := setWordOption(&options, flag, value); err != nil {
				return options, false, err
			}
		case arg == "--pdf-size" || arg == "--pdf-orientation" || arg == "--pdf-margin" || arg == "--pdf-header" || arg == "--pdf-footer":
			if i+1 >= len(args) {
				return options, false, fmt.Errorf("%s requires a value", arg)
			}
			i++
			if err := setPDFOption(&options, arg, args[i]); err != nil {
				return options, false, err
			}
		case strings.HasPrefix(arg, "--pdf-") && strings.Contains(arg, "="):
			parts := strings.SplitN(arg, "=", 2)
			if err := setPDFOption(&options, parts[0], parts[1]); err != nil {
				return options, false, err
			}
		case arg == "--pdf-no-page-numbers":
			options.pdf.PageNumbers = false
		case arg == "--pdf-no-bookmarks":
			options.pdf.Bookmarks = false
		case arg == "--pdf-break-h1":
			options.pdf.PageBreakBeforeH1 = true
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
	if options.format != "docx" && options.format != "pdf" {
		return options, false, fmt.Errorf("unsupported output format %q; supported: docx, pdf", options.format)
	}

	if options.output == "" {
		options.output = defaultOutputPath(options.input, options.format)
	}

	if options.format == "pdf" && (options.word != docx.Options{}) {
		return options, false, errors.New("Word options require DOCX output")
	}
	if options.word.Style == "" {
		options.word.Style = docx.StyleStandard
	}
	if options.word.TemplatePath != "" && options.word.Style != docx.StyleCustom {
		return options, false, errors.New("--word-template requires --word-style custom")
	}
	if options.word.Style == docx.StyleCustom && options.word.TemplatePath == "" {
		return options, false, errors.New("--word-style custom requires --word-template")
	}
	if options.word.Style != docx.StyleStandard && options.word.Style != docx.StyleNative && options.word.Style != docx.StyleCustom {
		return options, false, errors.New("unsupported --word-style")
	}
	if options.format == "pdf" {
		if _, err := pdf.Validate(options.pdf); err != nil {
			return options, false, err
		}
	}
	return options, false, nil
}

func setPDFOption(options *cliOptions, flag, value string) error {
	switch flag {
	case "--pdf-size":
		options.pdf.PageSize = value
	case "--pdf-orientation":
		options.pdf.Orientation = value
	case "--pdf-header":
		options.pdf.HeaderText = value
	case "--pdf-footer":
		options.pdf.FooterText = value
	case "--pdf-margin":
		margin, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("--pdf-margin requires a number in millimeters: %w", err)
		}
		options.pdf.MarginTop = margin
		options.pdf.MarginBottom = margin
		options.pdf.MarginRight = margin
		options.pdf.MarginLeft = margin
	default:
		return fmt.Errorf("unknown PDF option %q", flag)
	}
	return nil
}

func setWordOption(options *cliOptions, flag, value string) error {
	switch flag {
	case "--word-style":
		options.word.Style = docx.StyleMode(value)
	case "--word-template":
		options.word.TemplatePath = value
	case "--word-cover-title":
		options.word.CoverTitle = value
	case "--word-subtitle":
		options.word.Subtitle = value
	case "--word-company":
		options.word.Company = value
	case "--word-logo":
		options.word.LogoPath = value
	case "--word-header":
		options.word.HeaderText = value
	case "--word-footer":
		options.word.FooterText = value
	case "--word-paper":
		options.word.PaperSize = value
	default:
		return fmt.Errorf("unknown Word option %q", flag)
	}
	return nil
}
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "MDTranscode - turn Markdown into professional documents")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mdtranscode input.md --to docx")
	fmt.Fprintln(w, "  mdtranscode input.md --to pdf --pdf-size A4 --pdf-orientation landscape")
	fmt.Fprintln(w, "  mdtranscode input.md --to pdf --output report.pdf")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --to FORMAT             Output format: docx, pdf")
	fmt.Fprintln(w, "  -o, --output PATH       Explicit output path")
	fmt.Fprintln(w, "  --word-style MODE       standard (default), native, custom")
	fmt.Fprintln(w, "  --word-template FILE    Required with --word-style custom (.dotx)")
	fmt.Fprintln(w, "  --word-cover-title TXT  Optional title page")
	fmt.Fprintln(w, "  --word-subtitle TXT     Title page subtitle")
	fmt.Fprintln(w, "  --word-company NAME     Title page company")
	fmt.Fprintln(w, "  --word-logo FILE       Local cover logo image")
	fmt.Fprintln(w, "  --word-header TEXT      Word document header")
	fmt.Fprintln(w, "  --word-footer TEXT      Word document footer")
	fmt.Fprintln(w, "  --word-page-numbers     Add Word footer page numbers")
	fmt.Fprintln(w, "  --word-toc              Insert updatable Word TOC field")
	fmt.Fprintln(w, "  --word-break-h1         New page before each H1")
	fmt.Fprintln(w, "  --word-landscape        Landscape document orientation")
	fmt.Fprintln(w, "  --word-paper SIZE       Letter (default) or A4")
	fmt.Fprintln(w, "  --pdf-size SIZE         Letter or A4")
	fmt.Fprintln(w, "  --pdf-orientation DIR   portrait or landscape")
	fmt.Fprintln(w, "  --pdf-margin MM         Uniform margins (5..60 mm)")
	fmt.Fprintln(w, "  --pdf-header TEXT       Text in top page margin")
	fmt.Fprintln(w, "  --pdf-footer TEXT       Text in bottom page margin")
	fmt.Fprintln(w, "  --pdf-break-h1          Start each H1 section on a fresh page")
	fmt.Fprintln(w, "  --pdf-no-bookmarks      Do not create heading bookmarks")
	fmt.Fprintln(w, "  --pdf-no-page-numbers   Omit page numbers")
	fmt.Fprintln(w, "  -h, --help              Show this help")
}

func defaultOutputPath(input, format string) string {
	return transcode.DefaultOutputPath(input, format)
}

func convert(options cliOptions) error {
	_, err := transcode.ConvertFile(transcode.FileOptions{
		InputPath:  options.input,
		OutputPath: options.output,
		Format:     options.format,
		PDF:        options.pdf,
		DOCX:       options.word,
	})
	return err
}
