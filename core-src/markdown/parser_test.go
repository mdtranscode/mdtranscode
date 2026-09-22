package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdtranscode/mdtranscode/core-src/document"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "tests", "fixtures", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func TestSampleFixtureCoversPrototypeBlockTypes(t *testing.T) {
	doc := Parse(loadFixture(t, "sample.md"))
	if doc == nil || len(doc.Blocks) == 0 {
		t.Fatal("parser returned no blocks")
	}

	counts := map[document.BlockKind]int{}
	for _, block := range doc.Blocks {
		counts[block.Kind]++
	}

	for _, kind := range []document.BlockKind{
		document.BlockParagraph,
		document.BlockHeading,
		document.BlockCode,
		document.BlockListItem,
		document.BlockTable,
		document.BlockRule,
	} {
		if counts[kind] == 0 {
			t.Fatalf("fixture did not produce block kind %v", kind)
		}
	}
}

func TestSampleFixturePreservesHeadingAndHardBreak(t *testing.T) {
	doc := Parse(loadFixture(t, "sample.md"))

	if len(doc.Blocks) == 0 || doc.Blocks[0].Kind != document.BlockHeading || doc.Blocks[0].Level != 1 {
		t.Fatalf("first block is not H1: %#v", doc.Blocks[0])
	}
	if got := inlineText(doc.Blocks[0].Inlines); got != "Markdown → DOCX Test" {
		t.Fatalf("unexpected H1 text: %q", got)
	}

	foundBreak := false
	for _, block := range doc.Blocks {
		for _, inline := range block.Inlines {
			if inline.Kind == document.InlineHardBreak {
				foundBreak = true
			}
		}
	}
	if !foundBreak {
		t.Fatal("expected hard line break in sample fixture")
	}
}

func TestSampleFixturePreservesInlineFormatting(t *testing.T) {
	doc := Parse(loadFixture(t, "sample.md"))

	var all []document.Inline
	for _, block := range doc.Blocks {
		all = append(all, block.Inlines...)
		for _, row := range block.Rows {
			for _, cell := range row.Cells {
				all = append(all, cell.Inlines...)
			}
		}
	}

	assertInline := func(name string, fn func(document.Inline) bool) {
		t.Helper()
		for _, inline := range all {
			if fn(inline) {
				return
			}
		}
		t.Fatalf("missing inline behavior: %s", name)
	}

	assertInline("bold", func(x document.Inline) bool { return x.Bold && strings.Contains(x.Text, "bold") })
	assertInline("italic", func(x document.Inline) bool { return x.Italic && strings.Contains(x.Text, "italic") })
	assertInline("strike", func(x document.Inline) bool { return x.Strike && strings.Contains(x.Text, "strike") })
	assertInline("inline code", func(x document.Inline) bool { return x.Code && strings.Contains(x.Text, "inline code") })
	assertInline("link", func(x document.Inline) bool { return x.URL == "https://example.com" })
	assertInline("image", func(x document.Inline) bool { return x.Kind == document.InlineImage && x.ImageSource == "tiny.png" })
}

func TestTaskListIsSemantic(t *testing.T) {
	doc := Parse(loadFixture(t, "sample.md"))
	var checked, unchecked bool
	for _, block := range doc.Blocks {
		if block.Kind != document.BlockListItem || !block.Task {
			continue
		}
		if block.Checked && inlineText(block.Inlines) == "Completed task" {
			checked = true
		}
		if !block.Checked && inlineText(block.Inlines) == "Open task" {
			unchecked = true
		}
	}
	if !checked || !unchecked {
		t.Fatalf("task states not preserved: checked=%v unchecked=%v", checked, unchecked)
	}
}

func TestTableAlignmentAndEscapedPipe(t *testing.T) {
	doc := Parse(loadFixture(t, "sample.md"))
	for _, block := range doc.Blocks {
		if block.Kind != document.BlockTable {
			continue
		}
		if len(block.Alignments) != 3 || block.Alignments[0] != document.AlignLeft || block.Alignments[1] != document.AlignCenter || block.Alignments[2] != document.AlignRight {
			t.Fatalf("unexpected table alignments: %#v", block.Alignments)
		}
		if len(block.Rows) < 3 || len(block.Rows[2].Cells) < 1 {
			t.Fatal("expected sample table body rows")
		}
		if got := inlineText(block.Rows[2].Cells[0].Inlines); got != "x | y" {
			t.Fatalf("escaped table pipe not preserved: %q", got)
		}
		return
	}
	t.Fatal("table not found")
}

func TestReferenceLinksAndImages(t *testing.T) {
	doc := Parse(loadFixture(t, "refs.md"))
	var openAILinks int
	var image bool
	for _, block := range doc.Blocks {
		for _, inline := range block.Inlines {
			if inline.URL == "https://openai.com" {
				openAILinks++
			}
			if inline.Kind == document.InlineImage && inline.ImageSource == "tiny.png" && inline.Alt == "Logo" {
				image = true
			}
		}
	}
	if openAILinks != 2 {
		t.Fatalf("expected two reference links, got %d", openAILinks)
	}
	if !image {
		t.Fatal("reference image was not parsed")
	}
}

func TestInlineHTML(t *testing.T) {
	doc := Parse(loadFixture(t, "htmltest.md"))
	var bold, italic, underline, code, link, image bool
	for _, block := range doc.Blocks {
		for _, inline := range block.Inlines {
			bold = bold || (inline.Bold && strings.Contains(inline.Text, "bold & more"))
			italic = italic || (inline.Italic && inline.Text == "italic")
			underline = underline || (inline.Underline && inline.Text == "underlined")
			code = code || (inline.Code && inline.Text == "x < y")
			link = link || (inline.URL == "https://example.com" && inline.Text == "anchor")
			image = image || (inline.Kind == document.InlineImage && inline.ImageSource == "tiny.png" && inline.Alt == "tiny")
		}
	}
	if !bold || !italic || !underline || !code || !link || !image {
		t.Fatalf("inline HTML mismatch: bold=%v italic=%v underline=%v code=%v link=%v image=%v", bold, italic, underline, code, link, image)
	}
}

func TestBlockquoteLevel(t *testing.T) {
	doc := Parse(loadFixture(t, "sample.md"))
	var paragraph, list bool
	for _, block := range doc.Blocks {
		if block.QuoteLevel == 0 {
			continue
		}
		if block.Kind == document.BlockParagraph && strings.Contains(inlineText(block.Inlines), "Blockquote paragraph") {
			paragraph = true
		}
		if block.Kind == document.BlockListItem && strings.Contains(inlineText(block.Inlines), "Quoted list item") {
			list = true
		}
	}
	if !paragraph || !list {
		t.Fatalf("blockquote semantics missing: paragraph=%v list=%v", paragraph, list)
	}
}

func TestParserInstancesDoNotShareReferences(t *testing.T) {
	p1 := &Parser{}
	p2 := &Parser{}

	d1 := p1.Parse("[x][id]\n\n[id]: https://one.example\n")
	d2 := p2.Parse("[x][id]\n\n[id]: https://two.example\n")

	if got := firstURL(d1); got != "https://one.example" {
		t.Fatalf("parser 1 reference leaked or failed: %q", got)
	}
	if got := firstURL(d2); got != "https://two.example" {
		t.Fatalf("parser 2 reference leaked or failed: %q", got)
	}
}

func inlineText(inlines []document.Inline) string {
	var b strings.Builder
	for _, inline := range inlines {
		switch inline.Kind {
		case document.InlineText:
			b.WriteString(inline.Text)
		case document.InlineHardBreak:
			b.WriteString("\n")
		}
	}
	return b.String()
}

func firstURL(doc *document.Document) string {
	for _, block := range doc.Blocks {
		for _, inline := range block.Inlines {
			if inline.URL != "" {
				return inline.URL
			}
		}
	}
	return ""
}
