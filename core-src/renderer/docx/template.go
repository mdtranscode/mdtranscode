package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// wordTemplate represents the supported, deliberately narrow .dotx subset:
// Word paragraph/character styles and optional Office theme. Master pages,
// numbering, document body, macros and embedded objects are not imported.
// Nonempty text body and style numbering metadata are ignored with warnings.
type wordTemplate struct {
	styles   string
	theme    string
	warnings []string
}

func loadWordTemplate(path string) (*wordTemplate, error) {
	if !strings.EqualFold(filepath.Ext(path), ".dotx") {
		return nil, errors.New("Word template must have a .dotx extension")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("open Word template: %w", err)
	}
	if info.IsDir() || info.Size() > 32<<20 {
		return nil, errors.New("Word template is not a file or exceeds 32 MiB")
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("invalid .dotx ZIP: %w", err)
	}
	defer z.Close()
	if len(z.File) > 256 {
		return nil, errors.New("Word template contains too many parts")
	}
	parts := map[string][]byte{}
	var total uint64
	for _, f := range z.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || filepath.IsAbs(name) || strings.Contains("/"+name+"/", "/../") || strings.Contains("/"+name+"/", "/./") || strings.Contains(name, ":") {
			return nil, fmt.Errorf("unsafe template part path %q", name)
		}
		if _, ok := parts[name]; ok {
			return nil, fmt.Errorf("duplicate template part %s", name)
		}
		low := strings.ToLower(name)
		if strings.Contains(low, "vbaproject") || strings.Contains(low, "/activex/") || strings.Contains(low, "/embeddings/") || strings.Contains(low, "/oleobject") || strings.HasSuffix(low, ".bin") {
			return nil, fmt.Errorf("active content or embedded objects not permitted: %s", name)
		}
		total += f.UncompressedSize64
		if f.UncompressedSize64 > 8<<20 || total > 32<<20 {
			return nil, errors.New("template XML expansion limit exceeded")
		}
		// Only read XML/relationships; other parts are never copied into output.
		if !strings.HasSuffix(low, ".xml") && !strings.HasSuffix(low, ".rels") {
			parts[name] = nil
			continue
		}
		rc, e := f.Open()
		if e != nil {
			return nil, e
		}
		b, e := io.ReadAll(io.LimitReader(rc, 8<<20+1))
		rc.Close()
		if e != nil {
			return nil, e
		}
		if len(b) > 8<<20 {
			return nil, errors.New("template part exceeds 8 MiB")
		}
		if e = validatePartXML(b); e != nil {
			return nil, fmt.Errorf("malformed %s: %w", name, e)
		}
		parts[name] = b
	}
	content := string(parts["[Content_Types].xml"])
	if !strings.Contains(content, "wordprocessingml.template.main+xml") {
		return nil, errors.New("template content types do not identify a .dotx main document")
	}
	for name, data := range parts {
		if strings.HasSuffix(strings.ToLower(name), ".rels") {
			dec := xml.NewDecoder(bytes.NewReader(data))
			for {
				token, e := dec.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					return nil, e
				}
				if start, ok := token.(xml.StartElement); ok && start.Name.Local == "Relationship" {
					for _, a := range start.Attr {
						if a.Name.Local == "TargetMode" && strings.EqualFold(a.Value, "External") {
							return nil, fmt.Errorf("template has external relationship in %s", name)
						}
					}
				}
			}
		}
	}
	// Unsafe or unsupported embedded objects/master pages remain errors. Plain
	// body text is ignored with a visible warning: a .dotx is only a style/theme
	// source, not a Markdown document to merge into the export.
	for name := range parts {
		low := strings.ToLower(name)
		if strings.HasPrefix(low, "word/header") || strings.HasPrefix(low, "word/footer") {
			return nil, fmt.Errorf("template header/footer %s is not supported by style-only import", name)
		}
	}
	warnings := []string{}
	if body := parts["word/document.xml"]; len(body) > 0 {
		hasText := false
		d := xml.NewDecoder(bytes.NewReader(body))
		for {
			t, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, e
			}
			if start, ok := t.(xml.StartElement); ok {
				switch start.Name.Local {
				case "headerReference", "footerReference", "drawing", "pict", "tbl", "hyperlink", "fldSimple", "numPr":
					return nil, fmt.Errorf("template document content/master-page element %s is not supported", start.Name.Local)
				case "t":
					var value string
					if e := d.DecodeElement(&value, &start); e != nil {
						return nil, e
					}
					if strings.TrimSpace(value) != "" {
						hasText = true
					}
				}
			}
		}
		if hasText {
			warnings = append(warnings, "Template body text is not copied; only styles and theme are imported.")
		}
	}
	styles := parts["word/styles.xml"]
	if len(styles) == 0 {
		return nil, errors.New("template does not contain word/styles.xml")
	}
	// Word commonly places w:numPr on its built-in ListParagraph style even
	// when the document uses no list. Generated DOCX already owns numbering.xml
	// and its numIds. Remove only the template's style-level numbering metadata
	// to avoid dangling/colliding references while preserving all other styles.
	var strippedNumbering bool
	styles, strippedNumbering, err = stripStyleNumbering(styles)
	if err != nil {
		return nil, fmt.Errorf("invalid template style numbering: %w", err)
	}
	if strippedNumbering {
		warnings = append(warnings, "Template style numbering references are ignored; MDTranscode uses its own list numbering.")
	}
	ids := map[string]bool{}
	var basedOn []string
	dec := xml.NewDecoder(bytes.NewReader(styles))
	for {
		t, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		switch elem := t.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "style":
				for _, a := range elem.Attr {
					if a.Name.Local == "styleId" {
						if ids[a.Value] {
							return nil, fmt.Errorf("duplicate Word style ID %q", a.Value)
						}
						ids[a.Value] = true
					}
				}
			case "basedOn", "next", "link":
				for _, a := range elem.Attr {
					if a.Name.Local == "val" {
						basedOn = append(basedOn, a.Value)
					}
				}
			}
			for _, a := range elem.Attr {
				if a.Name.Local == "id" && a.Name.Space != "" || a.Name.Local == "embed" || a.Name.Local == "link" && a.Name.Space != "" {
					return nil, errors.New("template styles contain package relationship references")
				}
			}
		}
	}
	for _, n := range []string{"Normal", "Heading1", "Heading2", "Heading3", "Heading4", "Heading5", "Heading6"} {
		if !ids[n] {
			return nil, fmt.Errorf("template must define Word style %s", n)
		}
	}
	for _, n := range basedOn {
		if !ids[n] && n != "DefaultParagraphFont" {
			return nil, fmt.Errorf("template style refers to missing style %s", n)
		}
	}
	// The document renderer emits CodeBlock and Hyperlink. Install safe fallbacks
	// when the template does not define them, retaining its own definitions if present.
	add := ""
	if !ids["CodeBlock"] {
		add += `<w:style w:type="paragraph" w:styleId="CodeBlock"><w:name w:val="Code Block"/><w:basedOn w:val="Normal"/></w:style>`
	}
	if !ids["Hyperlink"] {
		add += `<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:u w:val="single"/></w:rPr></w:style>`
	}
	result := string(styles)
	if add != "" {
		idx := strings.LastIndex(result, "</w:styles>")
		if idx < 0 {
			return nil, errors.New("template styles use unsupported XML namespace prefix")
		}
		result = result[:idx] + add + result[idx:]
	}
	t := &wordTemplate{styles: result, warnings: warnings}
	if theme := parts["word/theme/theme1.xml"]; len(theme) > 0 {
		if err := validateOfficeTheme(theme); err != nil {
			return nil, fmt.Errorf("invalid Word template theme: %w", err)
		}
		t.theme = string(theme)
	}
	return t, nil
}

func validatePartXML(data []byte) error {
	if len(data) == 0 {
		return errors.New("empty XML")
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// ValidateTemplate checks whether a local .dotx is within the supported
// safe, styles-and-theme-only profile without converting a document.
func ValidateTemplate(path string) error {
	_, err := loadWordTemplate(path)
	return err
}

// TemplateWarnings returns nonfatal compatibility notes for a valid template.
// Unsupported embedded objects, unsafe relationships and malformed XML still
// result in errors and are never silently included in an exported document.
func TemplateWarnings(path string) ([]string, error) {
	t, err := loadWordTemplate(path)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), t.warnings...), nil
}

// stripStyleNumbering strips style-local OOXML numbering metadata from the
// original XML bytes, preserving namespaces, unrelated style definitions and
// Word extension attributes. The renderer generates its own numbering.xml.
func stripStyleNumbering(data []byte) ([]byte, bool, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	last := 0
	depth := 0
	changed := false
	var output bytes.Buffer
	for {
		begin := int(dec.InputOffset())
		token, err := dec.Token()
		end := int(dec.InputOffset())
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, err
		}
		if depth > 0 {
			switch token.(type) {
			case xml.StartElement:
				depth++
			case xml.EndElement:
				depth--
				if depth == 0 {
					last = end
				}
			}
			continue
		}
		if e, ok := token.(xml.StartElement); ok && (e.Name.Local == "numPr" || e.Name.Local == "numId" || e.Name.Local == "abstractNumId") && e.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
			output.Write(data[last:begin])
			changed = true
			depth = 1
		}
	}
	if depth != 0 {
		return nil, false, errors.New("unterminated style numbering reference")
	}
	if !changed {
		return data, false, nil
	}
	output.Write(data[last:])
	return output.Bytes(), true, nil
}

// validateOfficeTheme enforces the required DrawingML theme structure before
// importing it verbatim. Merely well-formed XML is not a valid Office theme:
// an empty <a:theme/> causes Microsoft Word to request document recovery.
// This is a structural preflight, not a complete ECMA-376 schema validator.
func validateOfficeTheme(data []byte) error {
	const drawingNS = "http://schemas.openxmlformats.org/drawingml/2006/main"
	type themeElement struct {
		XMLName  xml.Name
		Children []themeElement `xml:",any"`
	}
	var theme themeElement
	if err := xml.Unmarshal(data, &theme); err != nil {
		return err
	}
	if theme.XMLName.Space != drawingNS || theme.XMLName.Local != "theme" {
		return errors.New("theme root must be a DrawingML a:theme element")
	}
	find := func(parent themeElement, name string) (themeElement, bool) {
		for _, child := range parent.Children {
			if child.XMLName.Space == drawingNS && child.XMLName.Local == name {
				return child, true
			}
		}
		return themeElement{}, false
	}
	require := func(parent themeElement, name string) (themeElement, error) {
		if node, ok := find(parent, name); ok {
			return node, nil
		}
		return themeElement{}, fmt.Errorf("DrawingML theme is missing %s", name)
	}
	elements, err := require(theme, "themeElements")
	if err != nil {
		return err
	}
	colors, err := require(elements, "clrScheme")
	if err != nil {
		return err
	}
	for _, name := range []string{"dk1", "lt1", "dk2", "lt2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"} {
		color, err := require(colors, name)
		if err != nil {
			return err
		}
		if len(color.Children) == 0 {
			return fmt.Errorf("DrawingML theme %s has no color value", name)
		}
	}
	fonts, err := require(elements, "fontScheme")
	if err != nil {
		return err
	}
	for _, name := range []string{"majorFont", "minorFont"} {
		font, err := require(fonts, name)
		if err != nil {
			return err
		}
		if _, err := require(font, "latin"); err != nil {
			return err
		}
	}
	format, err := require(elements, "fmtScheme")
	if err != nil {
		return err
	}
	for _, name := range []string{"fillStyleLst", "lnStyleLst", "effectStyleLst", "bgFillStyleLst"} {
		styles, err := require(format, name)
		if err != nil {
			return err
		}
		if len(styles.Children) == 0 {
			return fmt.Errorf("DrawingML theme %s is empty", name)
		}
	}
	return nil
}
