// Package pdf prints the shared semantic document model to a paginated PDF
// using Chromium/Edge's built-in print engine. No document data is sent to a
// remote service: HTML and readable local images are embedded in a temporary
// local file, and the browser runs in headless mode with a temporary profile.
package pdf

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mdtranscode/mdtranscode/core-src/document"
	htmlrenderer "github.com/mdtranscode/mdtranscode/core-src/renderer/html"
)

// Options are the document print preferences. Millimeters are used for margins.
type Options struct {
	BaseDir           string  `json:"-"`
	Title             string  `json:"-"`
	PageSize          string  `json:"pageSize"`    // Letter or A4
	Orientation       string  `json:"orientation"` // portrait or landscape
	MarginTop         float64 `json:"marginTop"`
	MarginRight       float64 `json:"marginRight"`
	MarginBottom      float64 `json:"marginBottom"`
	MarginLeft        float64 `json:"marginLeft"`
	HeaderText        string  `json:"headerText"`
	FooterText        string  `json:"footerText"`
	PageNumbers       bool    `json:"pageNumbers"`
	Bookmarks         bool    `json:"bookmarks"`
	PageBreakBeforeH1 bool    `json:"pageBreakBeforeH1"`
	// BrowserPath is a controlled test/advanced deployment override. If empty,
	// Microsoft Edge is preferred on Windows, Chromium/Chrome on other hosts.
	BrowserPath string `json:"-"`
}

func Defaults() Options {
	return Options{PageSize: "Letter", Orientation: "portrait", MarginTop: 19, MarginRight: 19, MarginBottom: 19, MarginLeft: 19, PageNumbers: true, Bookmarks: true}
}

// Validate rejects settings that would create impossible or broken page layouts.
func Validate(o Options) (Options, error) {
	if o.PageSize == "" {
		o.PageSize = "Letter"
	}
	if o.Orientation == "" {
		o.Orientation = "portrait"
	}
	switch strings.ToLower(o.PageSize) {
	case "letter":
		o.PageSize = "Letter"
	case "a4":
		o.PageSize = "A4"
	default:
		return o, fmt.Errorf("page size must be Letter or A4")
	}
	switch strings.ToLower(o.Orientation) {
	case "portrait":
		o.Orientation = "portrait"
	case "landscape":
		o.Orientation = "landscape"
	default:
		return o, fmt.Errorf("orientation must be portrait or landscape")
	}
	w, h := 215.9, 279.4
	if o.PageSize == "A4" {
		w, h = 210, 297
	}
	if o.Orientation == "landscape" {
		w, h = h, w
	}
	for _, item := range []struct {
		name  string
		value float64
	}{{"top", o.MarginTop}, {"right", o.MarginRight}, {"bottom", o.MarginBottom}, {"left", o.MarginLeft}} {
		if item.value < 5 || item.value > 60 || item.value != item.value {
			return o, fmt.Errorf("%s PDF margin must be between 5 and 60 mm", item.name)
		}
	}
	if o.MarginLeft+o.MarginRight >= w-45 || o.MarginTop+o.MarginBottom >= h-45 {
		return o, errors.New("PDF margins leave too little printable area")
	}
	if len([]rune(o.HeaderText)) > 160 || len([]rune(o.FooterText)) > 160 {
		return o, errors.New("PDF header/footer must be 160 characters or fewer")
	}
	return o, nil
}

func cssContent(text string) string {
	// CSS string escaping, including linebreaks and control characters.
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range text {
		if r == '"' || r == '\\' {
			b.WriteRune('\\')
			b.WriteRune(r)
		} else if r < ' ' || r == 0x7f {
			b.WriteString("\\" + strconv.FormatInt(int64(r), 16) + " ")
		} else {
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// PrintHTML is a complete standalone local document consumed by the browser.
// The renderer's HTML generation escapes Markdown text and embeds local images.
func PrintHTML(doc *document.Document, o Options) (string, error) {
	var err error
	if o, err = Validate(o); err != nil {
		return "", err
	}
	result := htmlrenderer.Render(doc, htmlrenderer.Options{BaseDir: o.BaseDir, EmbedLocalImages: true, BlockRemoteImages: true})
	if doc == nil {
		return "", errors.New("document is required")
	}
	css := fmt.Sprintf(`@page { size: %s %s; margin: %.2fmm %.2fmm %.2fmm %.2fmm; }
`, o.PageSize, o.Orientation, o.MarginTop, o.MarginRight, o.MarginBottom, o.MarginLeft)
	// Chrome 131+ implements CSS @page margin boxes. The footer and running
	// page counter are printed in those boxes, never embedded in body text.
	if o.HeaderText != "" {
		css += fmt.Sprintf("@page { @top-center { content: %s; font: 9pt Arial,sans-serif; color: #555; } }\n", cssContent(o.HeaderText))
	}
	if o.FooterText != "" {
		css += fmt.Sprintf("@page { @bottom-left { content: %s; font: 9pt Arial,sans-serif; color: #555; } }\n", cssContent(o.FooterText))
	}
	if o.PageNumbers {
		css += "@page { @bottom-right { content: 'Page ' counter(page) ' of ' counter(pages); font: 9pt Arial,sans-serif; color: #555; } }\n"
	}
	if o.PageBreakBeforeH1 {
		css += "h1 ~ h1 { break-before: page; page-break-before: always; }\n"
	}
	css += `html { background: #fff; } body { margin: 0; color:#172030; font: 10.5pt/1.5 Arial,'Segoe UI',sans-serif; overflow-wrap: anywhere; }
 h1,h2,h3,h4,h5,h6 { line-height:1.25; color:#172b4d; break-after:avoid; page-break-after:avoid; margin:1em 0 .4em; }
 h1 { font-size:23pt; } h2 {font-size:17pt;} h3 {font-size:13pt;} h4 {font-size:11pt;} h5,h6 {font-size:10pt;}
 p {margin: .45em 0 .85em;} ul,ol {padding-left:1.6em;} li {margin:.15em 0;} blockquote {border-left:3px solid #879db6;margin:1em 0;padding:.15em 1em;color:#4d5a6c;}
 pre {background:#f4f6f8;padding:10px;border:1px solid #e3e6e9;border-radius:3px;white-space:pre-wrap;overflow-wrap:anywhere;break-inside:auto;font:9pt/1.45 Consolas,'Courier New',monospace;}
 code {font:9pt Consolas,'Courier New',monospace;overflow-wrap:anywhere;} :not(pre)>code {background:#f2f4f7;padding:1px 3px;}
 table {border-collapse:collapse;width:100%;font-size:9.5pt;margin:1em 0;} thead {display:table-header-group;} tr {break-inside:avoid;} th,td {border:1px solid #cbd3dd;padding:5px 7px;vertical-align:top;} th {background:#eef3f8;text-align:left;}
 img {max-width:100%;height:auto;break-inside:avoid;} a {color:#145a98;text-decoration:underline;}
 hr {border:0;border-top:1px solid #cbd3dd;margin:1.2em 0;} input[type=checkbox] {vertical-align:middle;}
 * {-webkit-print-color-adjust:exact;print-color-adjust:exact;}
 `
	return "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>" + html.EscapeString(o.Title) + "</title><style>" + css + "</style></head><body>" + result.BodyHTML + "</body></html>", nil
}

// WriteFile writes output only after Chrome/Edge has successfully produced a
// complete nonempty PDF. Existing output is never truncated on render failure.
func WriteFile(doc *document.Document, output string, o Options) error {
	if strings.TrimSpace(output) == "" {
		return errors.New("PDF output path is required")
	}
	htmlPage, err := PrintHTML(doc, o)
	if err != nil {
		return err
	}
	browser, err := findBrowser(o.BrowserPath)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "mdtranscode-pdf-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	pagePath := filepath.Join(tempDir, "document.html")
	pdfPath := filepath.Join(tempDir, "output.pdf")
	profile := filepath.Join(tempDir, "profile")
	if err = os.WriteFile(pagePath, []byte(htmlPage), 0600); err != nil {
		return fmt.Errorf("prepare PDF: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	// URL-escape spaces and Unicode in local paths before passing the file to Edge.
	urlPath := filepath.ToSlash(pagePath)
	if runtime.GOOS == "windows" {
		urlPath = "/" + strings.TrimPrefix(urlPath, "/")
	}
	fileURL := (&url.URL{Scheme: "file", Path: urlPath}).String()
	flags := []string{"--headless", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-sync", "--disable-extensions", "--host-resolver-rules=MAP * ~NOTFOUND", "--no-pdf-header-footer", "--user-data-dir=" + profile, "--print-to-pdf=" + pdfPath}
	if o.Bookmarks {
		flags = append(flags, "--generate-pdf-document-outline")
	}
	flags = append(flags, fileURL)
	cmd := exec.CommandContext(ctx, browser, flags...)
	log, runErr := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("PDF conversion timed out: %w", ctx.Err())
	}
	if runErr != nil {
		return fmt.Errorf("PDF conversion failed: %w (%s)", runErr, trimLog(log))
	}
	bytes, err := os.ReadFile(pdfPath)
	if err != nil || len(bytes) < 8 || !strings.HasPrefix(string(bytes), "%PDF-") {
		return fmt.Errorf("PDF engine did not produce a valid PDF: %s", trimLog(log))
	}
	if err := os.WriteFile(output, bytes, 0644); err != nil {
		return fmt.Errorf("save PDF %q: %w", output, err)
	}
	return nil
}
func trimLog(value []byte) string {
	s := strings.TrimSpace(string(value))
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
func findBrowser(explicit string) (string, error) {
	if explicit == "" {
		explicit = os.Getenv("MDTRANSCODE_PDF_BROWSER")
	}
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit, nil
		}
		if path, err := exec.LookPath(explicit); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("PDF browser not found at %q", explicit)
	}
	if runtime.GOOS == "windows" {
		for _, dir := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LocalAppData")} {
			if dir == "" {
				continue
			}
			for _, rel := range []string{filepath.Join("Microsoft", "Edge", "Application", "msedge.exe"), filepath.Join("Microsoft", "Edge Beta", "Application", "msedge.exe")} {
				path := filepath.Join(dir, rel)
				if _, err := os.Stat(path); err == nil {
					return path, nil
				}
			}
		}
	}
	for _, name := range []string{"msedge.exe", "msedge", "chromium", "chromium-browser", "google-chrome", "chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("PDF export requires Microsoft Edge (or a Chromium-compatible browser)")
}
