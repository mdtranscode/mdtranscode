package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mdtranscode/mdtranscode/core-src/document"
)

const (
	relHyperlink = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	relImage     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

// Options controls DOCX rendering. BaseDir is used to resolve relative image
// paths from the original Markdown document.
type Options struct {
	BaseDir string
	Title   string
	Creator string
}

type relationship struct {
	id     string
	typ    string
	target string
	mode   string
}

type mediaFile struct {
	name string
	path string
	data []byte
	cx   int64
	cy   int64
}

type builder struct {
	doc          *document.Document
	options      Options
	rels         []relationship
	media        []mediaFile
	relSeq       int
	imageSeq     int
	orderedNum   map[int]int
	orderedStart map[int]int
}

// Render converts a parsed document model into a DOCX package in memory.
func Render(doc *document.Document, options Options) ([]byte, error) {
	if doc == nil {
		return nil, errors.New("document is nil")
	}
	b := newBuilder(doc, options)
	return b.render()
}

// WriteFile converts a parsed document model into a DOCX file.
func WriteFile(doc *document.Document, outputPath string, options Options) error {
	data, err := Render(doc, options)
	if err != nil {
		return err
	}
	dir := filepath.Dir(outputPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("write DOCX: %w", err)
	}
	return nil
}

func newBuilder(doc *document.Document, options Options) *builder {
	if options.Title == "" {
		options.Title = "Converted Markdown"
	}
	if options.Creator == "" {
		options.Creator = "MDTranscode"
	}
	b := &builder{
		doc:          doc,
		options:      options,
		relSeq:       1,
		orderedNum:   make(map[int]int),
		orderedStart: make(map[int]int),
	}

	nextNumID := 10
	for _, block := range doc.Blocks {
		if block.Kind != document.BlockListItem || !block.Ordered || block.ListGroup <= 0 {
			continue
		}
		if _, exists := b.orderedNum[block.ListGroup]; exists {
			continue
		}
		b.orderedNum[block.ListGroup] = nextNumID
		b.orderedStart[block.ListGroup] = block.ListStart
		nextNumID++
	}
	return b
}

func (b *builder) render() ([]byte, error) {
	// documentXML resolves image and hyperlink relationships on demand, so it
	// must be built before relationship XML and media entries are written.
	documentXML, err := b.documentXML()
	if err != nil {
		return nil, err
	}

	var output bytes.Buffer
	zw := zip.NewWriter(&output)
	addText := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}

	parts := []struct {
		name    string
		content string
	}{
		{"[Content_Types].xml", b.contentTypesXML()},
		{"_rels/.rels", rootRelsXML()},
		{"docProps/core.xml", b.corePropsXML()},
		{"docProps/app.xml", appPropsXML()},
		{"word/document.xml", documentXML},
		{"word/styles.xml", stylesXML()},
		{"word/numbering.xml", b.numberingXML()},
		{"word/settings.xml", settingsXML()},
		{"word/_rels/document.xml.rels", b.documentRelsXML()},
	}
	for _, part := range parts {
		if err := addText(part.name, part.content); err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("write %s: %w", part.name, err)
		}
	}

	for _, media := range b.media {
		w, err := zw.Create("word/media/" + media.name)
		if err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("create media part: %w", err)
		}
		if _, err := w.Write(media.data); err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("write media part: %w", err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize DOCX: %w", err)
	}
	return output.Bytes(), nil
}

func (b *builder) addRel(typ, target, mode string) string {
	id := fmt.Sprintf("rId%d", b.relSeq)
	b.relSeq++
	b.rels = append(b.rels, relationship{id: id, typ: typ, target: target, mode: mode})
	return id
}

func (b *builder) addHyperlink(target string) string {
	return b.addRel(relHyperlink, target, "External")
}

func (b *builder) addImage(src string) (string, *mediaFile, error) {
	clean := src
	var data []byte
	var err error

	if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, requestErr := client.Get(clean)
		if requestErr != nil {
			return "", nil, requestErr
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", nil, fmt.Errorf("image download returned %s", resp.Status)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 25*1024*1024))
		if err != nil {
			return "", nil, err
		}
	} else {
		if parsed, parseErr := url.Parse(src); parseErr == nil && parsed.Scheme == "file" {
			clean = parsed.Path
		}
		if unescaped, unescapeErr := url.PathUnescape(clean); unescapeErr == nil {
			clean = unescaped
		}
		if strings.HasPrefix(clean, "data:") {
			return "", nil, errors.New("data URI images are not supported")
		}
		if !filepath.IsAbs(clean) {
			clean = filepath.Join(b.options.BaseDir, filepath.FromSlash(clean))
		}
		data, err = os.ReadFile(clean)
		if err != nil {
			return "", nil, err
		}
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", nil, err
	}
	ext := "." + format
	if format == "jpeg" {
		ext = ".jpg"
	}

	b.imageSeq++
	name := fmt.Sprintf("image%d%s", b.imageSeq, ext)

	// Assume 96 DPI and constrain images to 6 inches wide.
	cx := int64(cfg.Width) * 9525
	cy := int64(cfg.Height) * 9525
	maxCX := int64(6.0 * 914400)
	if cx > maxCX && cx > 0 {
		cy = cy * maxCX / cx
		cx = maxCX
	}

	media := mediaFile{name: name, path: clean, data: data, cx: cx, cy: cy}
	b.media = append(b.media, media)
	rid := b.addRel(relImage, "media/"+name, "")
	return rid, &b.media[len(b.media)-1], nil
}

func (b *builder) documentXML() (string, error) {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	out.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>`)

	for _, block := range b.doc.Blocks {
		switch block.Kind {
		case document.BlockHeading:
			xml, err := b.paragraphXML(block.Inlines, fmt.Sprintf("Heading%d", block.Level), block.QuoteLevel, 0, 0, "", false)
			if err != nil {
				return "", err
			}
			out.WriteString(xml)
		case document.BlockParagraph:
			xml, err := b.paragraphXML(block.Inlines, "", block.QuoteLevel, 0, 0, "", false)
			if err != nil {
				return "", err
			}
			out.WriteString(xml)
		case document.BlockCode:
			out.WriteString(b.codeXML(block.Text, block.QuoteLevel))
		case document.BlockListItem:
			numID := 1
			if block.Ordered {
				numID = b.orderedNum[block.ListGroup]
			}
			inlines := block.Inlines
			if block.Task {
				mark := "☐ "
				if block.Checked {
					mark = "☑ "
				}
				inlines = append([]document.Inline{{Kind: document.InlineText, Text: mark}}, inlines...)
			}
			xml, err := b.paragraphXML(inlines, "", block.QuoteLevel, numID, block.ListLevel, "", false)
			if err != nil {
				return "", err
			}
			out.WriteString(xml)
		case document.BlockTable:
			xml, err := b.tableXML(block)
			if err != nil {
				return "", err
			}
			out.WriteString(xml)
		case document.BlockRule:
			out.WriteString(ruleXML(block.QuoteLevel))
		}
	}

	out.WriteString(`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1080" w:right="1080" w:bottom="1080" w:left="1080" w:header="720" w:footer="720" w:gutter="0"/></w:sectPr></w:body></w:document>`)
	return out.String(), nil
}

func (b *builder) paragraphXML(inlines []document.Inline, style string, quoteLevel, numID, listLevel int, align string, forceBold bool) (string, error) {
	var out strings.Builder
	out.WriteString("<w:p><w:pPr>")
	if style != "" {
		out.WriteString(`<w:pStyle w:val="` + xmlEsc(style) + `"/>`)
	}
	if quoteLevel > 0 {
		left := 360 * quoteLevel
		out.WriteString(fmt.Sprintf(`<w:ind w:left="%d"/><w:pBdr><w:left w:val="single" w:sz="12" w:space="8" w:color="B7B7B7"/></w:pBdr>`, left))
	}
	if numID > 0 {
		if listLevel > 8 {
			listLevel = 8
		}
		out.WriteString(fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, listLevel, numID))
	}
	if align != "" {
		out.WriteString(`<w:jc w:val="` + xmlEsc(align) + `"/>`)
	}
	out.WriteString("</w:pPr>")

	inlineXML, err := b.inlinesXML(inlines, forceBold, true)
	if err != nil {
		return "", err
	}
	out.WriteString(inlineXML)
	out.WriteString("</w:p>")
	return out.String(), nil
}

func (b *builder) inlinesXML(inlines []document.Inline, forceBold, allowImages bool) (string, error) {
	var out strings.Builder
	for _, inline := range inlines {
		switch inline.Kind {
		case document.InlineHardBreak:
			out.WriteString(`<w:r><w:br/></w:r>`)
		case document.InlineImage:
			if allowImages {
				rid, media, err := b.addImage(inline.ImageSource)
				if err == nil && media != nil {
					out.WriteString(imageRunXML(rid, *media, b.imageSeq))
					continue
				}
			}
			alt := inline.Alt
			if alt == "" {
				alt = inline.ImageSource
			}
			out.WriteString(runXML("[Image: "+alt+"]", runStyle{italic: true, bold: forceBold}, false))
		case document.InlineText:
			style := runStyle{
				bold:      forceBold || inline.Bold,
				italic:    inline.Italic,
				strike:    inline.Strike,
				code:      inline.Code,
				underline: inline.Underline,
			}
			if inline.URL != "" {
				rid := b.addHyperlink(inline.URL)
				out.WriteString(`<w:hyperlink r:id="` + rid + `" w:history="1">` + runXML(inline.Text, style, true) + `</w:hyperlink>`)
			} else {
				out.WriteString(runXML(inline.Text, style, false))
			}
		}
	}
	return out.String(), nil
}

type runStyle struct {
	bold      bool
	italic    bool
	strike    bool
	code      bool
	underline bool
}

func runXML(text string, style runStyle, hyperlink bool) string {
	var out strings.Builder
	out.WriteString("<w:r><w:rPr>")
	if hyperlink {
		out.WriteString(`<w:rStyle w:val="Hyperlink"/>`)
	}
	if style.bold {
		out.WriteString(`<w:b/><w:bCs/>`)
	}
	if style.italic {
		out.WriteString(`<w:i/><w:iCs/>`)
	}
	if style.strike {
		out.WriteString(`<w:strike/>`)
	}
	if style.underline && !hyperlink {
		out.WriteString(`<w:u w:val="single"/>`)
	}
	if style.code {
		out.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:cs="Consolas"/><w:shd w:val="clear" w:fill="F2F2F2"/>`)
	}
	out.WriteString("</w:rPr>")

	parts := strings.Split(text, "\t")
	for i, part := range parts {
		if i > 0 {
			out.WriteString("<w:tab/>")
		}
		if part != "" {
			out.WriteString(`<w:t xml:space="preserve">` + xmlEsc(part) + `</w:t>`)
		}
	}
	out.WriteString("</w:r>")
	return out.String()
}

func (b *builder) codeXML(text string, quoteLevel int) string {
	var out strings.Builder
	out.WriteString(`<w:p><w:pPr><w:pStyle w:val="CodeBlock"/>`)
	if quoteLevel > 0 {
		out.WriteString(fmt.Sprintf(`<w:ind w:left="%d"/>`, 360*quoteLevel))
	}
	out.WriteString(`</w:pPr>`)
	for index, line := range strings.Split(text, "\n") {
		if index > 0 {
			out.WriteString(`<w:r><w:br/></w:r>`)
		}
		out.WriteString(runXML(line, runStyle{code: true}, false))
	}
	out.WriteString(`</w:p>`)
	return out.String()
}

func (b *builder) tableXML(table document.Block) (string, error) {
	if len(table.Rows) == 0 {
		return "", nil
	}
	columnCount := len(table.Rows[0].Cells)
	if columnCount == 0 {
		return "", nil
	}

	var out strings.Builder
	out.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblLayout w:type="autofit"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="BFBFBF"/><w:left w:val="single" w:sz="4" w:color="BFBFBF"/><w:bottom w:val="single" w:sz="4" w:color="BFBFBF"/><w:right w:val="single" w:sz="4" w:color="BFBFBF"/><w:insideH w:val="single" w:sz="4" w:color="D9D9D9"/><w:insideV w:val="single" w:sz="4" w:color="D9D9D9"/></w:tblBorders></w:tblPr>`)
	out.WriteString(`<w:tblGrid>`)
	for i := 0; i < columnCount; i++ {
		out.WriteString(`<w:gridCol w:w="2400"/>`)
	}
	out.WriteString(`</w:tblGrid>`)

	for rowIndex, row := range table.Rows {
		out.WriteString(`<w:tr>`)
		if rowIndex == 0 {
			out.WriteString(`<w:trPr><w:tblHeader w:val="on"/></w:trPr>`)
		}
		for columnIndex := 0; columnIndex < columnCount; columnIndex++ {
			cell := document.TableCell{}
			if columnIndex < len(row.Cells) {
				cell = row.Cells[columnIndex]
			}
			out.WriteString(`<w:tc><w:tcPr>`)
			if rowIndex == 0 {
				out.WriteString(`<w:shd w:val="clear" w:fill="EDEDED"/>`)
			}
			out.WriteString(`</w:tcPr>`)

			align := "left"
			if columnIndex < len(table.Alignments) && table.Alignments[columnIndex] != "" {
				align = string(table.Alignments[columnIndex])
			}
			// Prototype behavior rendered table-header images as text fallback. Keep
			// that behavior during migration; body cells can embed images.
			allowImages := rowIndex != 0
			paragraph, err := b.tableCellParagraphXML(cell.Inlines, align, rowIndex == 0, allowImages)
			if err != nil {
				return "", err
			}
			out.WriteString(paragraph)
			out.WriteString(`</w:tc>`)
		}
		out.WriteString(`</w:tr>`)
	}
	out.WriteString(`</w:tbl>`)
	return out.String(), nil
}

func (b *builder) tableCellParagraphXML(inlines []document.Inline, align string, forceBold, allowImages bool) (string, error) {
	var out strings.Builder
	out.WriteString(`<w:p><w:pPr><w:jc w:val="` + xmlEsc(align) + `"/></w:pPr>`)
	inlineXML, err := b.inlinesXML(inlines, forceBold, allowImages)
	if err != nil {
		return "", err
	}
	out.WriteString(inlineXML)
	out.WriteString(`</w:p>`)
	return out.String(), nil
}

func ruleXML(quoteLevel int) string {
	left := ""
	if quoteLevel > 0 {
		left = fmt.Sprintf(`<w:ind w:left="%d"/>`, 360*quoteLevel)
	}
	return `<w:p><w:pPr>` + left + `<w:pBdr><w:bottom w:val="single" w:sz="8" w:space="1" w:color="B7B7B7"/></w:pBdr></w:pPr><w:r><w:t></w:t></w:r></w:p>`
}

func imageRunXML(rid string, media mediaFile, id int) string {
	name := xmlEsc(filepath.Base(media.path))
	if name == "" {
		name = "Image"
	}
	return fmt.Sprintf(`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="%s"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`, media.cx, media.cy, id, name, name, rid, media.cx, media.cy)
}

func (b *builder) documentRelsXML() string {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	out.WriteString(`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`)
	out.WriteString(`<Relationship Id="rIdNumbering" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>`)
	out.WriteString(`<Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>`)
	for _, rel := range b.rels {
		out.WriteString(`<Relationship Id="` + rel.id + `" Type="` + rel.typ + `" Target="` + xmlEsc(rel.target) + `"`)
		if rel.mode != "" {
			out.WriteString(` TargetMode="` + xmlEsc(rel.mode) + `"`)
		}
		out.WriteString(`/>`)
	}
	out.WriteString(`</Relationships>`)
	return out.String()
}

func (b *builder) numberingXML() string {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	out.WriteString(`<w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="multilevel"/>`)
	for level := 0; level < 9; level++ {
		left := 720 + level*360
		out.WriteString(fmt.Sprintf(`<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/><w:pPr><w:tabs><w:tab w:val="num" w:pos="%d"/></w:tabs><w:ind w:left="%d" w:hanging="360"/></w:pPr><w:rPr><w:rFonts w:ascii="Arial" w:hAnsi="Arial"/></w:rPr></w:lvl>`, level, left, left))
	}
	out.WriteString(`</w:abstractNum><w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="multilevel"/>`)
	for level := 0; level < 9; level++ {
		left := 720 + level*360
		lvlText := "%" + strconv.Itoa(level+1) + "."
		out.WriteString(fmt.Sprintf(`<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%s"/><w:lvlJc w:val="left"/><w:pPr><w:tabs><w:tab w:val="num" w:pos="%d"/></w:tabs><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`, level, lvlText, left, left))
	}
	out.WriteString(`</w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`)

	groups := make([]int, 0, len(b.orderedNum))
	for group := range b.orderedNum {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return b.orderedNum[groups[i]] < b.orderedNum[groups[j]]
	})
	for _, group := range groups {
		id := b.orderedNum[group]
		start := b.orderedStart[group]
		out.WriteString(fmt.Sprintf(`<w:num w:numId="%d"><w:abstractNumId w:val="1"/>`, id))
		if start > 1 {
			out.WriteString(fmt.Sprintf(`<w:lvlOverride w:ilvl="0"><w:startOverride w:val="%d"/></w:lvlOverride>`, start))
		}
		out.WriteString(`</w:num>`)
	}
	out.WriteString(`</w:numbering>`)
	return out.String()
}

func (b *builder) contentTypesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="jpg" ContentType="image/jpeg"/><Default Extension="jpeg" ContentType="image/jpeg"/><Default Extension="gif" ContentType="image/gif"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/><Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`
}

func rootRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`
}

func (b *builder) corePropsXML() string {
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>` + xmlEsc(b.options.Title) + `</dc:title><dc:creator>` + xmlEsc(b.options.Creator) + `</dc:creator><cp:lastModifiedBy>` + xmlEsc(b.options.Creator) + `</cp:lastModifiedBy><dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified></cp:coreProperties>`
}

func appPropsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>Microsoft Office Word</Application><AppVersion>16.0000</AppVersion></Properties>`
}

func settingsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:zoom w:val="bestFit"/><w:defaultTabStop w:val="720"/><w:characterSpacingControl w:val="doNotCompress"/><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/><w:compatSetting w:name="overrideTableStyleFontSizeAndJustification" w:uri="http://schemas.microsoft.com/office/word" w:val="1"/><w:compatSetting w:name="enableOpenTypeFeatures" w:uri="http://schemas.microsoft.com/office/word" w:val="1"/><w:compatSetting w:name="doNotFlipMirrorIndents" w:uri="http://schemas.microsoft.com/office/word" w:val="1"/></w:compat><w:themeFontLang w:val="en-US"/><w:decimalSymbol w:val="."/><w:listSeparator w:val=","/></w:settings>`
}

func stylesXML() string {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Aptos" w:hAnsi="Aptos" w:eastAsia="Aptos" w:cs="Aptos"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>`)
	out.WriteString(`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>`)
	sizes := []int{34, 30, 28, 26, 24, 22}
	for index, size := range sizes {
		out.WriteString(fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="Heading%d"><w:name w:val="heading %d"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="240" w:after="120"/><w:outlineLvl w:val="%d"/></w:pPr><w:rPr><w:b/><w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr></w:style>`, index+1, index+1, index, size, size))
	}
	out.WriteString(`<w:style w:type="paragraph" w:styleId="CodeBlock"><w:name w:val="Code Block"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="80" w:after="80" w:line="240" w:lineRule="auto"/><w:ind w:left="240" w:right="240"/><w:shd w:val="clear" w:fill="F5F5F5"/><w:pBdr><w:top w:val="single" w:sz="4" w:color="D9D9D9"/><w:left w:val="single" w:sz="4" w:color="D9D9D9"/><w:bottom w:val="single" w:sz="4" w:color="D9D9D9"/><w:right w:val="single" w:sz="4" w:color="D9D9D9"/></w:pBdr></w:pPr><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="19"/><w:szCs w:val="19"/></w:rPr></w:style>`)
	out.WriteString(`<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:basedOn w:val="DefaultParagraphFont"/><w:uiPriority w:val="99"/><w:unhideWhenUsed/><w:rPr><w:color w:val="0563C1"/><w:u w:val="single"/></w:rPr></w:style></w:styles>`)
	return out.String()
}

func xmlEsc(value string) string {
	var clean strings.Builder
	for _, r := range value {
		if r == 0x9 || r == 0xA || r == 0xD || r >= 0x20 {
			clean.WriteRune(r)
		}
	}
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(clean.String()))
	return out.String()
}
