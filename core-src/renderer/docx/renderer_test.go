package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdtranscode/mdtranscode/core-src/document"
	"github.com/mdtranscode/mdtranscode/core-src/markdown"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "tests", "fixtures", name)
}

func renderSample(t *testing.T) []byte {
	t.Helper()
	inputPath := fixturePath("sample.md")
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read sample fixture: %v", err)
	}
	doc := markdown.Parse(string(data))
	result, err := Render(doc, Options{
		BaseDir: filepath.Dir(inputPath),
		Title:   "Renderer Test",
		Creator: "MDTranscode Tests",
	})
	if err != nil {
		t.Fatalf("render sample: %v", err)
	}
	return result
}

func openPackage(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open DOCX package: %v", err)
	}
	parts := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open part %s: %v", file.Name, err)
		}
		content := new(bytes.Buffer)
		if _, err := content.ReadFrom(rc); err != nil {
			_ = rc.Close()
			t.Fatalf("read part %s: %v", file.Name, err)
		}
		_ = rc.Close()
		parts[file.Name] = content.Bytes()
	}
	return parts
}

func requirePart(t *testing.T, parts map[string][]byte, name string) string {
	t.Helper()
	content, ok := parts[name]
	if !ok {
		t.Fatalf("missing DOCX part %s", name)
	}
	return string(content)
}

func TestRenderProducesModernDOCXPackage(t *testing.T) {
	parts := openPackage(t, renderSample(t))

	required := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"docProps/core.xml",
		"docProps/app.xml",
		"word/document.xml",
		"word/styles.xml",
		"word/numbering.xml",
		"word/settings.xml",
		"word/_rels/document.xml.rels",
	}
	for _, name := range required {
		requirePart(t, parts, name)
	}

	settings := requirePart(t, parts, "word/settings.xml")
	if !strings.Contains(settings, `w:name="compatibilityMode"`) || !strings.Contains(settings, `w:val="15"`) {
		t.Fatal("modern Word compatibilityMode=15 is missing")
	}

	contentTypes := requirePart(t, parts, "[Content_Types].xml")
	if !strings.Contains(contentTypes, `/word/settings.xml`) || !strings.Contains(contentTypes, `wordprocessingml.settings+xml`) {
		t.Fatal("settings content type is missing")
	}

	rels := requirePart(t, parts, "word/_rels/document.xml.rels")
	if !strings.Contains(rels, `Id="rIdSettings"`) || !strings.Contains(rels, `/relationships/settings`) {
		t.Fatal("settings relationship is missing")
	}
}

func TestRenderedXMLPartsAreWellFormed(t *testing.T) {
	parts := openPackage(t, renderSample(t))
	for name, content := range parts {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		decoder := xml.NewDecoder(bytes.NewReader(content))
		for {
			_, err := decoder.Token()
			if err == nil {
				continue
			}
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("invalid XML in %s: %v", name, err)
		}
	}
}

func TestRendererPreservesNativeWordStructures(t *testing.T) {
	parts := openPackage(t, renderSample(t))
	docXML := requirePart(t, parts, "word/document.xml")
	numbering := requirePart(t, parts, "word/numbering.xml")
	rels := requirePart(t, parts, "word/_rels/document.xml.rels")

	checks := map[string]string{
		"heading style":          `w:pStyle w:val="Heading1"`,
		"table":                  `<w:tbl>`,
		"repeating table header": `<w:tblHeader w:val="on"/>`,
		"code block style":       `w:pStyle w:val="CodeBlock"`,
		"hyperlink":              `<w:hyperlink`,
		"hard break":             `<w:br/>`,
		"image drawing":          `<w:drawing>`,
		"checked task":           `☑ `,
		"unchecked task":         `☐ `,
		"blockquote border":      `<w:left w:val="single"`,
		"center table alignment": `w:jc w:val="center"`,
		"right table alignment":  `w:jc w:val="right"`,
	}
	for label, needle := range checks {
		if !strings.Contains(docXML, needle) {
			t.Fatalf("missing %s (%s)", label, needle)
		}
	}

	if !strings.Contains(numbering, `<w:startOverride w:val="5"/>`) {
		t.Fatal("ordered-list start value 5 was not preserved")
	}
	if !strings.Contains(rels, `Target="https://example.com"`) || !strings.Contains(rels, `TargetMode="External"`) {
		t.Fatal("external hyperlink relationship is missing")
	}
	if !strings.Contains(rels, `/relationships/image`) {
		t.Fatal("image relationship is missing")
	}

	mediaFound := false
	for name := range parts {
		if strings.HasPrefix(name, "word/media/") {
			mediaFound = true
			break
		}
	}
	if !mediaFound {
		t.Fatal("embedded image media part is missing")
	}
}

func TestRendererDoesNotNeedMarkdownSource(t *testing.T) {
	doc := &document.Document{Blocks: []document.Block{
		{
			Kind:  document.BlockHeading,
			Level: 2,
			Inlines: []document.Inline{
				{Kind: document.InlineText, Text: "Already parsed ", Bold: true},
				{Kind: document.InlineText, Text: "document model", URL: "https://example.com"},
			},
		},
	}}
	result, err := Render(doc, Options{})
	if err != nil {
		t.Fatalf("render document model: %v", err)
	}
	parts := openPackage(t, result)
	docXML := requirePart(t, parts, "word/document.xml")
	if !strings.Contains(docXML, `Heading2`) || !strings.Contains(docXML, `<w:b/>`) || !strings.Contains(docXML, `<w:hyperlink`) {
		t.Fatal("renderer did not consume document-model semantics directly")
	}
}

func TestMissingImageFallsBackToUsefulText(t *testing.T) {
	doc := &document.Document{Blocks: []document.Block{
		{
			Kind: document.BlockParagraph,
			Inlines: []document.Inline{
				{Kind: document.InlineImage, ImageSource: "does-not-exist.png", Alt: "Missing diagram"},
			},
		},
	}}
	result, err := Render(doc, Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("missing image should fall back rather than fail rendering: %v", err)
	}
	parts := openPackage(t, result)
	if !strings.Contains(requirePart(t, parts, "word/document.xml"), `[Image: Missing diagram]`) {
		t.Fatal("missing-image fallback text not found")
	}
}

func TestWriteFile(t *testing.T) {
	doc := &document.Document{Blocks: []document.Block{
		{Kind: document.BlockParagraph, Inlines: []document.Inline{{Kind: document.InlineText, Text: "Hello"}}},
	}}
	path := filepath.Join(t.TempDir(), "nested", "output.docx")
	if err := WriteFile(doc, path, Options{}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("output not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output DOCX is empty")
	}
}

func TestWriteVerificationSample(t *testing.T) {
	output := os.Getenv("MDTRANSCODE_VERIFY_DOCX")
	if output == "" {
		t.Skip("MDTRANSCODE_VERIFY_DOCX is not set")
	}
	data := renderSample(t)
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		t.Fatalf("create verification output directory: %v", err)
	}
	if err := os.WriteFile(output, data, 0644); err != nil {
		t.Fatalf("write verification DOCX: %v", err)
	}
}
