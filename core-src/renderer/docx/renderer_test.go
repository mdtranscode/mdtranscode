package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
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

// Iteration 4 Stage 1: Standard is the zero-value mode and stays compatible.
func TestStyleStandardIsUnchangedByExplicitSelection(t *testing.T) {
	doc := markdown.Parse("# Heading\n\nBody.\n\n- A\n- B\n\n| One | Two |\n|---|---|\n| x | y |\n")
	implicit, err := Render(doc, Options{Title: "Same"})
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := Render(doc, Options{Title: "Same", Style: StyleStandard})
	if err != nil {
		t.Fatal(err)
	}
	a, b := openPackage(t, implicit), openPackage(t, explicit)
	if len(a) != len(b) {
		t.Fatalf("part counts differ: %d vs %d", len(a), len(b))
	}
	for path, content := range a {
		if !bytes.Equal(content, b[path]) {
			t.Errorf("Standard part differs: %s", path)
		}
	}
}

func TestNativeMinimalDOCXHasSemanticStylesWithoutForcedBranding(t *testing.T) {
	doc := markdown.Parse("# Heading\n\nA paragraph.\n\n> A quote.\n\n| A | B |\n|---|---|\n| x | y |\n\n```go\nfmt.Println(42)\n```\n")
	output, err := Render(doc, Options{Style: StyleNative})
	if err != nil {
		t.Fatal(err)
	}
	parts := openPackage(t, output)
	styles := requirePart(t, parts, "word/styles.xml")
	if !strings.Contains(styles, `w:styleId="Heading1"`) || !strings.Contains(styles, `w:styleId="CodeBlock"`) || !strings.Contains(styles, `w:styleId="Normal"`) {
		t.Fatal("native semantic styles are missing")
	}
	if strings.Contains(styles, `w:ascii="Aptos"`) || strings.Contains(styles, `w:sz w:val="34"`) || strings.Contains(styles, `F5F5F5`) {
		t.Fatal("native styles unexpectedly hard-code Standard appearance")
	}
	document := requirePart(t, parts, "word/document.xml")
	if !strings.Contains(document, `<w:tbl>`) || !strings.Contains(document, `w:pStyle w:val="Heading1"`) || !strings.Contains(document, `w:pStyle w:val="CodeBlock"`) {
		t.Fatal("native DOCX lost semantic document structure")
	}
	if strings.Contains(document, `<w:tblBorders>`) || strings.Contains(document, `w:fill="EDEDED"`) {
		t.Fatal("native DOCX contains hard-coded Standard table decorations")
	}
	if !strings.Contains(requirePart(t, parts, "word/settings.xml"), `w:val="15"`) {
		t.Fatal("native DOCX lost modern Word compatibility")
	}
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
			t.Fatalf("%s invalid XML: %v", name, err)
		}
	}
}

func TestWordStyleModesRejectUnknownAndUnimplemented(t *testing.T) {
	doc := markdown.Parse("# Hello")
	for _, mode := range []StyleMode{StyleCustom, "unknown", "NATIVE"} {
		result, err := Render(doc, Options{Style: mode})
		if err == nil || result != nil {
			t.Errorf("mode %q should fail closed: err=%v", mode, err)
		}
	}
}

// testOfficeThemeXML is a complete, schema-shaped Word theme fixture. The
// earlier empty <a:theme/> test fixture was well-formed but invalid in Word.
const testOfficeThemeXML = `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Example Brand">
  <a:themeElements>
    <a:clrScheme name="Office">
      <a:dk1>
        <a:sysClr val="windowText" lastClr="000000"/>
      </a:dk1>
      <a:lt1>
        <a:sysClr val="window" lastClr="FFFFFF"/>
      </a:lt1>
      <a:dk2>
        <a:srgbClr val="1F497D"/>
      </a:dk2>
      <a:lt2>
        <a:srgbClr val="EEECE1"/>
      </a:lt2>
      <a:accent1>
        <a:srgbClr val="4F81BD"/>
      </a:accent1>
      <a:accent2>
        <a:srgbClr val="C0504D"/>
      </a:accent2>
      <a:accent3>
        <a:srgbClr val="9BBB59"/>
      </a:accent3>
      <a:accent4>
        <a:srgbClr val="8064A2"/>
      </a:accent4>
      <a:accent5>
        <a:srgbClr val="4BACC6"/>
      </a:accent5>
      <a:accent6>
        <a:srgbClr val="F79646"/>
      </a:accent6>
      <a:hlink>
        <a:srgbClr val="0000FF"/>
      </a:hlink>
      <a:folHlink>
        <a:srgbClr val="800080"/>
      </a:folHlink>
    </a:clrScheme>
    <a:fontScheme name="Office">
      <a:majorFont>
        <a:latin typeface="Calibri"/>
        <a:ea typeface=""/>
        <a:cs typeface=""/>
        <a:font script="Jpan" typeface="ＭＳ ゴシック"/>
        <a:font script="Hang" typeface="맑은 고딕"/>
        <a:font script="Hans" typeface="宋体"/>
        <a:font script="Hant" typeface="新細明體"/>
        <a:font script="Arab" typeface="Times New Roman"/>
        <a:font script="Hebr" typeface="Times New Roman"/>
        <a:font script="Thai" typeface="Angsana New"/>
        <a:font script="Ethi" typeface="Nyala"/>
        <a:font script="Beng" typeface="Vrinda"/>
        <a:font script="Gujr" typeface="Shruti"/>
        <a:font script="Khmr" typeface="MoolBoran"/>
        <a:font script="Knda" typeface="Tunga"/>
        <a:font script="Guru" typeface="Raavi"/>
        <a:font script="Cans" typeface="Euphemia"/>
        <a:font script="Cher" typeface="Plantagenet Cherokee"/>
        <a:font script="Yiii" typeface="Microsoft Yi Baiti"/>
        <a:font script="Tibt" typeface="Microsoft Himalaya"/>
        <a:font script="Thaa" typeface="MV Boli"/>
        <a:font script="Deva" typeface="Mangal"/>
        <a:font script="Telu" typeface="Gautami"/>
        <a:font script="Taml" typeface="Latha"/>
        <a:font script="Syrc" typeface="Estrangelo Edessa"/>
        <a:font script="Orya" typeface="Kalinga"/>
        <a:font script="Mlym" typeface="Kartika"/>
        <a:font script="Laoo" typeface="DokChampa"/>
        <a:font script="Sinh" typeface="Iskoola Pota"/>
        <a:font script="Mong" typeface="Mongolian Baiti"/>
        <a:font script="Viet" typeface="Times New Roman"/>
        <a:font script="Uigh" typeface="Microsoft Uighur"/>
        <a:font script="Geor" typeface="Sylfaen"/>
      </a:majorFont>
      <a:minorFont>
        <a:latin typeface="Cambria"/>
        <a:ea typeface=""/>
        <a:cs typeface=""/>
        <a:font script="Jpan" typeface="ＭＳ 明朝"/>
        <a:font script="Hang" typeface="맑은 고딕"/>
        <a:font script="Hans" typeface="宋体"/>
        <a:font script="Hant" typeface="新細明體"/>
        <a:font script="Arab" typeface="Arial"/>
        <a:font script="Hebr" typeface="Arial"/>
        <a:font script="Thai" typeface="Cordia New"/>
        <a:font script="Ethi" typeface="Nyala"/>
        <a:font script="Beng" typeface="Vrinda"/>
        <a:font script="Gujr" typeface="Shruti"/>
        <a:font script="Khmr" typeface="DaunPenh"/>
        <a:font script="Knda" typeface="Tunga"/>
        <a:font script="Guru" typeface="Raavi"/>
        <a:font script="Cans" typeface="Euphemia"/>
        <a:font script="Cher" typeface="Plantagenet Cherokee"/>
        <a:font script="Yiii" typeface="Microsoft Yi Baiti"/>
        <a:font script="Tibt" typeface="Microsoft Himalaya"/>
        <a:font script="Thaa" typeface="MV Boli"/>
        <a:font script="Deva" typeface="Mangal"/>
        <a:font script="Telu" typeface="Gautami"/>
        <a:font script="Taml" typeface="Latha"/>
        <a:font script="Syrc" typeface="Estrangelo Edessa"/>
        <a:font script="Orya" typeface="Kalinga"/>
        <a:font script="Mlym" typeface="Kartika"/>
        <a:font script="Laoo" typeface="DokChampa"/>
        <a:font script="Sinh" typeface="Iskoola Pota"/>
        <a:font script="Mong" typeface="Mongolian Baiti"/>
        <a:font script="Viet" typeface="Arial"/>
        <a:font script="Uigh" typeface="Microsoft Uighur"/>
        <a:font script="Geor" typeface="Sylfaen"/>
      </a:minorFont>
    </a:fontScheme>
    <a:fmtScheme name="Office">
      <a:fillStyleLst>
        <a:solidFill>
          <a:schemeClr val="phClr"/>
        </a:solidFill>
        <a:gradFill rotWithShape="1">
          <a:gsLst>
            <a:gs pos="0">
              <a:schemeClr val="phClr">
                <a:tint val="50000"/>
                <a:satMod val="300000"/>
              </a:schemeClr>
            </a:gs>
            <a:gs pos="35000">
              <a:schemeClr val="phClr">
                <a:tint val="37000"/>
                <a:satMod val="300000"/>
              </a:schemeClr>
            </a:gs>
            <a:gs pos="100000">
              <a:schemeClr val="phClr">
                <a:tint val="15000"/>
                <a:satMod val="350000"/>
              </a:schemeClr>
            </a:gs>
          </a:gsLst>
          <a:lin ang="16200000" scaled="1"/>
        </a:gradFill>
        <a:gradFill rotWithShape="1">
          <a:gsLst>
            <a:gs pos="0">
              <a:schemeClr val="phClr">
                <a:tint val="100000"/>
                <a:shade val="100000"/>
                <a:satMod val="130000"/>
              </a:schemeClr>
            </a:gs>
            <a:gs pos="100000">
              <a:schemeClr val="phClr">
                <a:tint val="50000"/>
                <a:shade val="100000"/>
                <a:satMod val="350000"/>
              </a:schemeClr>
            </a:gs>
          </a:gsLst>
          <a:lin ang="16200000" scaled="0"/>
        </a:gradFill>
      </a:fillStyleLst>
      <a:lnStyleLst>
        <a:ln w="9525" cap="flat" cmpd="sng" algn="ctr">
          <a:solidFill>
            <a:schemeClr val="phClr">
              <a:shade val="95000"/>
              <a:satMod val="105000"/>
            </a:schemeClr>
          </a:solidFill>
          <a:prstDash val="solid"/>
        </a:ln>
        <a:ln w="25400" cap="flat" cmpd="sng" algn="ctr">
          <a:solidFill>
            <a:schemeClr val="phClr"/>
          </a:solidFill>
          <a:prstDash val="solid"/>
        </a:ln>
        <a:ln w="38100" cap="flat" cmpd="sng" algn="ctr">
          <a:solidFill>
            <a:schemeClr val="phClr"/>
          </a:solidFill>
          <a:prstDash val="solid"/>
        </a:ln>
      </a:lnStyleLst>
      <a:effectStyleLst>
        <a:effectStyle>
          <a:effectLst>
            <a:outerShdw blurRad="40000" dist="20000" dir="5400000" rotWithShape="0">
              <a:srgbClr val="000000">
                <a:alpha val="38000"/>
              </a:srgbClr>
            </a:outerShdw>
          </a:effectLst>
        </a:effectStyle>
        <a:effectStyle>
          <a:effectLst>
            <a:outerShdw blurRad="40000" dist="23000" dir="5400000" rotWithShape="0">
              <a:srgbClr val="000000">
                <a:alpha val="35000"/>
              </a:srgbClr>
            </a:outerShdw>
          </a:effectLst>
        </a:effectStyle>
        <a:effectStyle>
          <a:effectLst>
            <a:outerShdw blurRad="40000" dist="23000" dir="5400000" rotWithShape="0">
              <a:srgbClr val="000000">
                <a:alpha val="35000"/>
              </a:srgbClr>
            </a:outerShdw>
          </a:effectLst>
          <a:scene3d>
            <a:camera prst="orthographicFront">
              <a:rot lat="0" lon="0" rev="0"/>
            </a:camera>
            <a:lightRig rig="threePt" dir="t">
              <a:rot lat="0" lon="0" rev="1200000"/>
            </a:lightRig>
          </a:scene3d>
          <a:sp3d>
            <a:bevelT w="63500" h="25400"/>
          </a:sp3d>
        </a:effectStyle>
      </a:effectStyleLst>
      <a:bgFillStyleLst>
        <a:solidFill>
          <a:schemeClr val="phClr"/>
        </a:solidFill>
        <a:gradFill rotWithShape="1">
          <a:gsLst>
            <a:gs pos="0">
              <a:schemeClr val="phClr">
                <a:tint val="40000"/>
                <a:satMod val="350000"/>
              </a:schemeClr>
            </a:gs>
            <a:gs pos="40000">
              <a:schemeClr val="phClr">
                <a:tint val="45000"/>
                <a:shade val="99000"/>
                <a:satMod val="350000"/>
              </a:schemeClr>
            </a:gs>
            <a:gs pos="100000">
              <a:schemeClr val="phClr">
                <a:shade val="20000"/>
                <a:satMod val="255000"/>
              </a:schemeClr>
            </a:gs>
          </a:gsLst>
          <a:path path="circle">
            <a:fillToRect l="50000" t="-80000" r="50000" b="180000"/>
          </a:path>
        </a:gradFill>
        <a:gradFill rotWithShape="1">
          <a:gsLst>
            <a:gs pos="0">
              <a:schemeClr val="phClr">
                <a:tint val="80000"/>
                <a:satMod val="300000"/>
              </a:schemeClr>
            </a:gs>
            <a:gs pos="100000">
              <a:schemeClr val="phClr">
                <a:shade val="30000"/>
                <a:satMod val="200000"/>
              </a:schemeClr>
            </a:gs>
          </a:gsLst>
          <a:path path="circle">
            <a:fillToRect l="50000" t="50000" r="50000" b="50000"/>
          </a:path>
        </a:gradFill>
      </a:bgFillStyleLst>
    </a:fmtScheme>
  </a:themeElements>
  <a:objectDefaults>
    <a:spDef>
      <a:spPr/>
      <a:bodyPr/>
      <a:lstStyle/>
      <a:style>
        <a:lnRef idx="1">
          <a:schemeClr val="accent1"/>
        </a:lnRef>
        <a:fillRef idx="3">
          <a:schemeClr val="accent1"/>
        </a:fillRef>
        <a:effectRef idx="2">
          <a:schemeClr val="accent1"/>
        </a:effectRef>
        <a:fontRef idx="minor">
          <a:schemeClr val="lt1"/>
        </a:fontRef>
      </a:style>
    </a:spDef>
    <a:lnDef>
      <a:spPr/>
      <a:bodyPr/>
      <a:lstStyle/>
      <a:style>
        <a:lnRef idx="2">
          <a:schemeClr val="accent1"/>
        </a:lnRef>
        <a:fillRef idx="0">
          <a:schemeClr val="accent1"/>
        </a:fillRef>
        <a:effectRef idx="1">
          <a:schemeClr val="accent1"/>
        </a:effectRef>
        <a:fontRef idx="minor">
          <a:schemeClr val="tx1"/>
        </a:fontRef>
      </a:style>
    </a:lnDef>
  </a:objectDefaults>
  <a:extraClrSchemeLst/>
</a:theme>
`

func buildTestTemplate(t *testing.T, additions map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "corporate.dotx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	entries := map[string]string{
		"[Content_Types].xml":   `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.template.main+xml"/></Types>`,
		"word/styles.xml":       nativeStylesXML(),
		"word/document.xml":     `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
		"word/theme/theme1.xml": testOfficeThemeXML,
	}
	for name, value := range additions {
		entries[name] = value
	}
	for name, value := range entries {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCustomWordTemplateImportsStylesAndTheme(t *testing.T) {
	template := buildTestTemplate(t, nil)
	doc := markdown.Parse("# Heading\n\nHello company\n")
	data, err := Render(doc, Options{Style: StyleCustom, TemplatePath: template})
	if err != nil {
		t.Fatal(err)
	}
	parts := openPackage(t, data)
	if !strings.Contains(requirePart(t, parts, "word/styles.xml"), `Heading1`) {
		t.Fatal("template styles absent")
	}
	if !strings.Contains(requirePart(t, parts, "word/theme/theme1.xml"), `Example Brand`) {
		t.Fatal("theme absent")
	}
	if !strings.Contains(requirePart(t, parts, "word/_rels/document.xml.rels"), `Target="theme/theme1.xml"`) {
		t.Fatal("theme relationship missing")
	}
	for name, contents := range parts {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			if err := validatePartXML(contents); err != nil {
				t.Fatalf("invalid %s: %v", name, err)
			}
		}
	}
}

func TestWordBrandingGeneratedPartsAndOptions(t *testing.T) {
	doc := markdown.Parse("# Hello\n\nWorld")
	data, err := Render(doc, Options{CoverTitle: "Report & Research", Subtitle: "Q4", Company: "Acme", LogoPath: fixturePath("tiny.png"), HeaderText: "Internal", FooterText: "Confidential", PageNumbers: true, TOC: true, BreakBeforeH1: true, Landscape: true, PaperSize: "A4"})
	if err != nil {
		t.Fatal(err)
	}
	parts := openPackage(t, data)
	xml := requirePart(t, parts, "word/document.xml")
	for _, term := range []string{"Report &amp; Research", "Table of Contents", "w:instrText", `w:orient="landscape"`, `w:headerReference`, `w:footerReference`, `w:type="page"`, `w:drawing`} {
		if !strings.Contains(xml, term) {
			t.Errorf("missing document feature %s", term)
		}
	}
	requirePart(t, parts, "word/header1.xml")
	if !strings.Contains(requirePart(t, parts, "word/footer1.xml"), `fldCharType="begin"`) {
		t.Fatal("page number field missing")
	}
	for name, content := range parts {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			if err := validatePartXML(content); err != nil {
				t.Fatalf("invalid XML %s: %v", name, err)
			}
		}
	}
}

func TestCustomWordTemplateRejectsInvalidPackages(t *testing.T) {
	for name, overrides := range map[string]map[string]string{
		"bad styles":      {"word/styles.xml": `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`},
		"external":        {"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="example" Target="https://example.test" TargetMode="External"/></Relationships>`},
		"active content":  {"word/vbaProject.bin": "BAD"},
		"invalid XML":     {"word/theme/theme1.xml": "<theme"},
		"empty theme":     {"word/theme/theme1.xml": `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Bad"/>`},
		"missing colors":  {"word/theme/theme1.xml": strings.Replace(testOfficeThemeXML, "<a:clrScheme name=", "<a:badScheme name=", 1)},
		"missing fonts":   {"word/theme/theme1.xml": strings.Replace(testOfficeThemeXML, "<a:fontScheme name=", "<a:badScheme name=", 1)},
		"missing formats": {"word/theme/theme1.xml": strings.Replace(testOfficeThemeXML, "<a:fmtScheme name=", "<a:badScheme name=", 1)},
		"unsafe path":     {"../outside.xml": "<bad/>"},
		"duplicate style": {"word/styles.xml": strings.Replace(nativeStylesXML(), `</w:styles>`, `<w:style w:type="paragraph" w:styleId="Normal"/></w:styles>`, 1)},
	} {
		t.Run(name, func(t *testing.T) {
			path := buildTestTemplate(t, overrides)
			if err := ValidateTemplate(path); err == nil {
				t.Fatal("invalid template accepted")
			}
		})
	}
	if _, err := Render(markdown.Parse("# h"), Options{Style: StyleCustom}); err == nil {
		t.Fatal("missing template accepted")
	}
	if _, err := Render(markdown.Parse("# h"), Options{TemplatePath: "foo.dotx"}); err == nil {
		t.Fatal("template with Standard accepted")
	}
}

// Regression for user-observed empty TOC/blank page and oversized cover logo.
// The image dimensions are intentionally much larger than a cover should allow.
func TestCoverLogoTOCAndPageBreakLayout(t *testing.T) {
	logo := filepath.Join(t.TempDir(), "oversize.png")
	img := image.NewRGBA(image.Rect(0, 0, 1800, 1800))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	f, err := os.Create(logo)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, landscape := range []bool{false, true} {
		data, err := Render(markdown.Parse("# Report & Review\n\n## Background\n\nIntro text.\n\n# Next Stage\n\n## Details"), Options{
			CoverTitle: "Report", LogoPath: logo, TOC: true,
			BreakBeforeH1: true, Landscape: landscape,
		})
		if err != nil {
			t.Fatal(err)
		}
		parts := openPackage(t, data)
		xml := requirePart(t, parts, "word/document.xml")
		settings := requirePart(t, parts, "word/settings.xml")
		if strings.Contains(settings, `w:updateFields`) {
			t.Fatal("TOC output must not force Word to update fields on open")
		}
		if strings.Count(xml, `w:type="page"`) != 3 { // cover, TOC, between H1 sections
			t.Fatalf("landscape=%v: want exactly three page breaks, got %d", landscape, strings.Count(xml, `w:type="page"`))
		}
		firstHeading := strings.Index(xml, `w:pStyle w:val="Heading1"`)
		if firstHeading < 0 {
			t.Fatal("first heading missing")
		}
		beforeFirstHeading := xml[:firstHeading]
		if strings.Count(beforeFirstHeading, `w:type="page"`) != 2 {
			t.Fatal("unexpected extra page break before first body heading")
		}
		for _, want := range []string{`w:instrText`, `w:fldCharType="separate"`, `w:fldCharType="end"`, `Report &amp; Review`, `Background`, `Next Stage`, `Details`} {
			if !strings.Contains(beforeFirstHeading, want) {
				t.Fatalf("missing cached TOC feature %q", want)
			}
		}
		const extent = `<wp:extent cx="1143000" cy="1143000"/>`
		if !strings.Contains(xml, extent) {
			t.Fatalf("oversized logo was not limited to 1.25 inches, missing %s", extent)
		}
		for name, contents := range parts {
			if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
				if err := validatePartXML(contents); err != nil {
					t.Fatalf("invalid %s: %v", name, err)
				}
			}
		}
	}
}

// A user observed Word's "fields may refer to other files" prompt when
// opening a custom-template export with TOC enabled. Avoid the automatic
// field-refresh setting while keeping the editable TOC and cached headings.
func TestTOCDoesNotForceFieldRefreshOnOpen(t *testing.T) {
	for _, style := range []StyleMode{StyleStandard, StyleNative, StyleCustom} {
		for _, toc := range []bool{false, true} {
			options := Options{Style: style, TOC: toc, PageNumbers: true}
			if style == StyleCustom {
				options.TemplatePath = buildTestTemplate(t, nil)
			}
			data, err := Render(markdown.Parse("# First\n\n## Second"), options)
			if err != nil {
				t.Fatalf("style=%q toc=%v: %v", style, toc, err)
			}
			parts := openPackage(t, data)
			if strings.Contains(requirePart(t, parts, "word/settings.xml"), `w:updateFields`) {
				t.Fatalf("style=%q toc=%v: automatic update setting must be absent", style, toc)
			}
			docXML := requirePart(t, parts, "word/document.xml")
			if toc {
				for _, want := range []string{` TOC \o "1-3" \h \z \u `, `First`, `Second`, `w:fldCharType="end"`} {
					if !strings.Contains(docXML, want) {
						t.Fatalf("style=%q missing TOC field/cache %q", style, want)
					}
				}
			} else if strings.Contains(docXML, ` TOC \o `) {
				t.Fatalf("style=%q: unexpected TOC when disabled", style)
			}
			if !strings.Contains(requirePart(t, parts, "word/footer1.xml"), ` PAGE `) {
				t.Fatalf("style=%q: lost normal PAGE field", style)
			}
		}
	}
}

func TestFirstH1DoesNotCreateBlankFirstPage(t *testing.T) {
	doc := markdown.Parse("# First heading\n\nText\n\n# Second heading")
	data, err := Render(doc, Options{BreakBeforeH1: true})
	if err != nil {
		t.Fatal(err)
	}
	xml := requirePart(t, openPackage(t, data), "word/document.xml")
	if strings.Count(xml, `w:type="page"`) != 1 {
		t.Fatalf("expected a break only before second H1")
	}
	data, err = Render(doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(requirePart(t, openPackage(t, data), "word/settings.xml"), "updateFields") {
		t.Fatal("Standard settings changed without TOC")
	}
}

func TestCoverLogoKeepsWideAspectRatio(t *testing.T) {
	logo := filepath.Join(t.TempDir(), "wide.png")
	img := image.NewRGBA(image.Rect(0, 0, 1600, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 1600; x++ {
			img.Set(x, y, color.White)
		}
	}
	f, err := os.Create(logo)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	data, err := Render(markdown.Parse("# Report"), Options{LogoPath: logo})
	if err != nil {
		t.Fatal(err)
	}
	xml := requirePart(t, openPackage(t, data), "word/document.xml")
	if !strings.Contains(xml, `<wp:extent cx="2743200" cy="342900"/>`) {
		t.Fatal("wide cover logo should be width-limited and retain aspect ratio")
	}
}

// Word-created templates may contain sample body text and built-in ListParagraph
// numbering references. Style-only import warns and strips unsafe numbering,
// rather than forcing users to remove normal Word styles manually.
func TestWordCreatedTemplateContentAndStyleNumberingAreNonfatal(t *testing.T) {
	body := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Example template instructions</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`
	modifiedStyles := strings.Replace(nativeStylesXML(), `</w:styles>`, `<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:pPr><w:numPr><w:numId w:val="99"/></w:numPr></w:pPr></w:style></w:styles>`, 1)
	template := buildTestTemplate(t, map[string]string{"word/document.xml": body, "word/styles.xml": modifiedStyles})
	warnings, err := TemplateWarnings(template)
	if err != nil {
		t.Fatalf("nonfatal Word template details caused a rejection: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("want 2 warnings for ignored text and numbering; got %v", warnings)
	}
	if err = ValidateTemplate(template); err != nil {
		t.Fatal(err)
	}
	data, err := Render(markdown.Parse("# Output heading\n\n- First\n- Second\n"), Options{Style: StyleCustom, TemplatePath: template, TOC: true})
	if err != nil {
		t.Fatal(err)
	}
	parts := openPackage(t, data)
	document := requirePart(t, parts, "word/document.xml")
	if strings.Contains(document, "Example template instructions") {
		t.Fatal("template body text leaked into rendered output")
	}
	styles := requirePart(t, parts, "word/styles.xml")
	if !strings.Contains(styles, `w:styleId="ListParagraph"`) {
		t.Fatal("Word style was discarded")
	}
	if strings.Contains(styles, `<w:numPr>`) || strings.Contains(styles, `w:val="99"`) {
		t.Fatal("orphan template numbering copied into Word style")
	}
	if err := validatePartXML([]byte(styles)); err != nil {
		t.Fatal(err)
	}
	numbering := requirePart(t, parts, "word/numbering.xml")
	if !strings.Contains(numbering, `w:numId="1"`) {
		t.Fatal("MDTranscode generated numbering was not retained")
	}
}

func TestWordTemplateStillRejectsEmbeddedObjects(t *testing.T) {
	body := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:drawing/></w:r></w:p></w:body></w:document>`
	template := buildTestTemplate(t, map[string]string{"word/document.xml": body})
	if _, err := TemplateWarnings(template); err == nil {
		t.Fatal("unsafe drawing accepted as a warning")
	}
}
