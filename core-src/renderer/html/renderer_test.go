package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdtranscode/mdtranscode/core-src/document"
	"github.com/mdtranscode/mdtranscode/core-src/markdown"
)

func htmlFixturePath(name string) string {
	return filepath.Join("..", "..", "tests", "fixtures", name)
}

func TestRenderSampleProducesSemanticHTMLAndOutline(t *testing.T) {
	inputPath := htmlFixturePath("sample.md")
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read sample fixture: %v", err)
	}

	result := Render(markdown.Parse(string(data)), Options{
		BaseDir:          filepath.Dir(inputPath),
		EmbedLocalImages: true,
	})

	checks := map[string]string{
		"heading":              `<h1 id="markdown-docx-test">`,
		"bold":                 `<strong>bold</strong>`,
		"italic":               `<em>italic</em>`,
		"strike":               `<del>strike</del>`,
		"inline code":          `<code>inline code</code>`,
		"safe hyperlink":       `href="https://example.com"`,
		"hard break":           `<br>`,
		"unordered list":       `<ul>`,
		"ordered list start":   `<ol start="5">`,
		"checked task":         `<input type="checkbox" disabled checked`,
		"unchecked task":       `<input type="checkbox" disabled aria-hidden`,
		"blockquote":           `<blockquote>`,
		"table":                `<table>`,
		"center alignment":     `style="text-align:center"`,
		"right alignment":      `style="text-align:right"`,
		"fenced code":          `function hello($name)`,
		"embedded local image": `src="data:image/png;base64,`,
		"autolink":             `href="https://example.org"`,
		"mailto":               `href="mailto:person@example.org"`,
	}
	for label, needle := range checks {
		if !strings.Contains(result.BodyHTML, needle) {
			t.Fatalf("missing %s (%s) in rendered HTML:\n%s", label, needle, result.BodyHTML)
		}
	}

	if len(result.Outline) < 5 {
		t.Fatalf("outline has %d items, expected multiple headings", len(result.Outline))
	}
	if result.Outline[0].ID != "markdown-docx-test" || result.Outline[0].Level != 1 || result.Outline[0].Text != "Markdown → DOCX Test" {
		t.Fatalf("unexpected first outline item: %#v", result.Outline[0])
	}
}

func TestRenderConsumesDocumentModelWithoutMarkdownParsing(t *testing.T) {
	doc := &document.Document{Blocks: []document.Block{
		{
			Kind:  document.BlockHeading,
			Level: 2,
			Inlines: []document.Inline{
				{Kind: document.InlineText, Text: "Already parsed ", Bold: true},
				{Kind: document.InlineText, Text: "model", URL: "https://example.com"},
			},
		},
	}}

	result := Render(doc, Options{})
	if !strings.Contains(result.BodyHTML, `<h2 id="already-parsed-model">`) || !strings.Contains(result.BodyHTML, `<strong>Already parsed </strong>`) {
		t.Fatalf("renderer did not consume model semantics directly: %s", result.BodyHTML)
	}
	if len(result.Outline) != 1 || result.Outline[0].Text != "Already parsed model" {
		t.Fatalf("unexpected outline: %#v", result.Outline)
	}
}

func TestRenderEscapesTextAndRejectsUnsafeLinks(t *testing.T) {
	doc := &document.Document{Blocks: []document.Block{
		{
			Kind: document.BlockParagraph,
			Inlines: []document.Inline{
				{Kind: document.InlineText, Text: `<script>alert("x")</script>`},
				{Kind: document.InlineText, Text: " unsafe", URL: "javascript:alert(1)"},
			},
		},
	}}

	result := Render(doc, Options{})
	if strings.Contains(result.BodyHTML, "<script>") || strings.Contains(result.BodyHTML, "javascript:") {
		t.Fatalf("unsafe content was emitted: %s", result.BodyHTML)
	}
	if !strings.Contains(result.BodyHTML, `&lt;script&gt;`) {
		t.Fatalf("text was not escaped: %s", result.BodyHTML)
	}
}

func TestRenderSupportedHTMLInlineSemantics(t *testing.T) {
	inputPath := htmlFixturePath("htmltest.md")
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read HTML inline fixture: %v", err)
	}

	result := Render(markdown.Parse(string(data)), Options{
		BaseDir:          filepath.Dir(inputPath),
		EmbedLocalImages: true,
	})

	checks := []string{
		`<strong>bold &amp; more</strong>`,
		`<em>italic</em>`,
		`<u>underlined</u>`,
		`<code>x &lt; y</code>`,
		`href="https://example.com"`,
		`src="data:image/png;base64,`,
	}
	for _, needle := range checks {
		if !strings.Contains(result.BodyHTML, needle) {
			t.Fatalf("missing %s in rendered HTML: %s", needle, result.BodyHTML)
		}
	}
}

func TestRenderDuplicateHeadingsGetUniqueIDs(t *testing.T) {
	doc := markdown.Parse("# Same\n\n## Same\n\n# !!!\n\n# !!!\n")
	result := Render(doc, Options{})

	want := []string{"same", "same-2", "section", "section-2"}
	if len(result.Outline) != len(want) {
		t.Fatalf("outline = %#v", result.Outline)
	}
	for i, id := range want {
		if result.Outline[i].ID != id {
			t.Fatalf("outline[%d].ID = %q, want %q", i, result.Outline[i].ID, id)
		}
	}
}

func TestMissingLocalImageUsesFallbackText(t *testing.T) {
	doc := &document.Document{Blocks: []document.Block{
		{
			Kind: document.BlockParagraph,
			Inlines: []document.Inline{
				{Kind: document.InlineImage, ImageSource: "missing.png", Alt: "Missing diagram"},
			},
		},
	}}

	result := Render(doc, Options{BaseDir: t.TempDir(), EmbedLocalImages: true})
	if !strings.Contains(result.BodyHTML, `[Image: Missing diagram]`) {
		t.Fatalf("missing-image fallback not found: %s", result.BodyHTML)
	}
}
