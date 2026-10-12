package pdf

import (
	"github.com/mdtranscode/mdtranscode/core-src/markdown"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrintHTMLPaginationSettingsAndEscaping(t *testing.T) {
	doc := markdown.Parse("# First\n\nRésumé & café 日本語\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n```\n<unsafe>\n```\n\n# Second\n\nMore text")
	o := Defaults()
	o.PageSize = "A4"
	o.Orientation = "landscape"
	o.MarginTop = 15
	o.HeaderText = `Company "quoted" <private>`
	o.FooterText = "Confidential"
	o.PageBreakBeforeH1 = true
	o.Title = "Business Report"
	html, err := PrintHTML(doc, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`size: A4 landscape`, `15.00mm`, `@top-center`, `Company \"quoted\" <private>`, `@bottom-left`, `Confidential`, `counter(page)`, `counter(pages)`, `h1 ~ h1`, `<table>`, `&lt;unsafe&gt;`, `Résumé &amp; café 日本語`, `id="first"`, `id="second"`} {
		if !strings.Contains(html, want) {
			t.Errorf("PDF HTML missing %q", want)
		}
	}
	if strings.Contains(html, `<unsafe>`) {
		t.Fatal("Markdown code was not escaped")
	}
}
func TestValidateOptions(t *testing.T) {
	for _, change := range []func(*Options){func(o *Options) { o.PageSize = "legal" }, func(o *Options) { o.Orientation = "diagonal" }, func(o *Options) { o.MarginTop = 0 }, func(o *Options) { o.MarginLeft = 61 }} {
		o := Defaults()
		change(&o)
		if _, err := Validate(o); err == nil {
			t.Errorf("expected invalid layout rejected: %+v", o)
		}
	}
	o := Defaults()
	o.PageSize = "a4"
	o.Orientation = "LANDSCAPE"
	got, err := Validate(o)
	if err != nil || got.PageSize != "A4" || got.Orientation != "landscape" {
		t.Fatalf("valid normalization failed: %+v, %v", got, err)
	}
}
func TestPDFDoesNotDestroyExistingFileOnBrowserFailure(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "original.pdf")
	if err := os.WriteFile(out, []byte("keep-original"), 0600); err != nil {
		t.Fatal(err)
	}
	o := Defaults()
	o.BrowserPath = filepath.Join(dir, "missing-browser")
	if err := WriteFile(markdown.Parse("# Test"), out, o); err == nil {
		t.Fatal("expected missing browser error")
	}
	after, err := os.ReadFile(out)
	if err != nil || string(after) != "keep-original" {
		t.Fatal("existing output was modified on PDF failure")
	}
}
func TestHeadlessBrowserArgumentsAndLocalSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell mock only; real Windows Edge is tested separately")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-browser.sh")
	pdfOut := filepath.Join(dir, "result.pdf")
	// Test stub models the browser output contract and inspects flags. This
	// does NOT substitute for a real Edge rendering acceptance test on Windows.
	source := `#!/bin/sh
for arg in "$@"; do
 case "$arg" in
 --print-to-pdf=*) output="${arg#--print-to-pdf=}" ;;
 --generate-pdf-document-outline) outline=yes ;;
 file://*) page="$arg" ;;
 esac
done
[ "$outline" = yes ] || exit 7
[ -n "$page" ] || exit 8
printf '%%PDF-1.7\nmock\n' > "$output"
`
	if err := os.WriteFile(script, []byte(source), 0700); err != nil {
		t.Fatal(err)
	}
	options := Defaults()
	options.BrowserPath = script
	if err := WriteFile(markdown.Parse("# Alpha\n\nParagraph"), pdfOut, options); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(pdfOut)
	if err != nil || !strings.HasPrefix(string(bytes), "%PDF-") {
		t.Fatalf("PDF output contract not met: %v", err)
	}
}

func TestRemoteImagesAreNotFetched(t *testing.T) {
	doc := markdown.Parse("# Report\n\n![remote](https://example.org/private-image.png)\n")
	page, err := PrintHTML(doc, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, "example.org/private-image.png") {
		t.Fatal("PDF source retained a remote resource URL")
	}
	if !strings.Contains(page, "[Image: remote]") {
		t.Fatal("PDF should render a local missing-image placeholder")
	}
}
