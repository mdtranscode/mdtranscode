package html

import (
	"encoding/base64"
	stdhtml "html"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/mdtranscode/mdtranscode/core-src/document"
)

// Options controls target-neutral document-model rendering to semantic HTML.
type Options struct {
	// BaseDir resolves relative local image paths.
	BaseDir string

	// EmbedLocalImages converts readable local images to data URLs. This is
	// useful for self-contained application previews that cannot safely rely on
	// direct file:// access.
	EmbedLocalImages bool
}

// OutlineItem describes one rendered heading and its stable in-document anchor.
type OutlineItem struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// Result contains the semantic HTML body fragment and heading outline produced
// from one already-parsed document model.
type Result struct {
	BodyHTML string        `json:"bodyHtml"`
	Outline  []OutlineItem `json:"outline"`
}

// Render converts an already-parsed document model to semantic HTML. It never
// reparses Markdown source.
func Render(doc *document.Document, options Options) Result {
	if doc == nil {
		return Result{}
	}

	r := renderer{
		options:    options,
		headingIDs: make(map[string]int),
	}
	r.renderBlocks(doc.Blocks)
	return Result{BodyHTML: r.out.String(), Outline: r.outline}
}

type renderer struct {
	options    Options
	out        strings.Builder
	outline    []OutlineItem
	headingIDs map[string]int
	quoteLevel int
}

func (r *renderer) renderBlocks(blocks []document.Block) {
	for i := 0; i < len(blocks); {
		block := blocks[i]
		r.setQuoteLevel(block.QuoteLevel)

		if block.Kind == document.BlockListItem {
			end := i + 1
			for end < len(blocks) && blocks[end].Kind == document.BlockListItem && blocks[end].QuoteLevel == block.QuoteLevel {
				end++
			}
			r.renderListRun(blocks[i:end])
			i = end
			continue
		}

		r.renderBlock(block)
		i++
	}
	r.setQuoteLevel(0)
}

func (r *renderer) setQuoteLevel(target int) {
	if target < 0 {
		target = 0
	}
	for r.quoteLevel < target {
		r.out.WriteString("<blockquote>\n")
		r.quoteLevel++
	}
	for r.quoteLevel > target {
		r.out.WriteString("</blockquote>\n")
		r.quoteLevel--
	}
}

func (r *renderer) renderBlock(block document.Block) {
	switch block.Kind {
	case document.BlockParagraph:
		r.out.WriteString("<p>")
		r.renderInlines(block.Inlines)
		r.out.WriteString("</p>\n")
	case document.BlockHeading:
		level := block.Level
		if level < 1 || level > 6 {
			level = 1
		}
		text := plainText(block.Inlines)
		id := r.uniqueHeadingID(text)
		r.outline = append(r.outline, OutlineItem{ID: id, Level: level, Text: text})
		r.out.WriteString("<h")
		r.out.WriteString(strconv.Itoa(level))
		r.out.WriteString(` id="`)
		r.out.WriteString(stdhtml.EscapeString(id))
		r.out.WriteString(`">`)
		r.renderInlines(block.Inlines)
		r.out.WriteString("</h")
		r.out.WriteString(strconv.Itoa(level))
		r.out.WriteString(">\n")
	case document.BlockCode:
		r.out.WriteString("<pre><code>")
		r.out.WriteString(stdhtml.EscapeString(block.Text))
		r.out.WriteString("</code></pre>\n")
	case document.BlockTable:
		r.renderTable(block)
	case document.BlockRule:
		r.out.WriteString("<hr>\n")
	}
}

func (r *renderer) renderListRun(blocks []document.Block) {
	if len(blocks) == 0 {
		return
	}

	pos := 0
	for pos < len(blocks) {
		level := blocks[pos].ListLevel
		if level < 0 {
			level = 0
		}
		r.renderListLevel(blocks, &pos, level, blocks[pos].Ordered)
	}
}

func (r *renderer) renderListLevel(blocks []document.Block, pos *int, level int, ordered bool) {
	if *pos >= len(blocks) {
		return
	}

	first := blocks[*pos]
	if ordered {
		r.out.WriteString("<ol")
		if first.ListStart > 1 {
			r.out.WriteString(` start="`)
			r.out.WriteString(strconv.Itoa(first.ListStart))
			r.out.WriteString(`"`)
		}
		r.out.WriteString(">\n")
	} else {
		r.out.WriteString("<ul>\n")
	}

	expected := first.ListStart
	if expected <= 0 {
		expected = 1
	}

	for *pos < len(blocks) {
		block := blocks[*pos]
		blockLevel := block.ListLevel
		if blockLevel < 0 {
			blockLevel = 0
		}
		if blockLevel < level || blockLevel > level || block.Ordered != ordered {
			break
		}

		r.out.WriteString("<li")
		if ordered && block.ListStart > 0 && block.ListStart != expected {
			r.out.WriteString(` value="`)
			r.out.WriteString(strconv.Itoa(block.ListStart))
			r.out.WriteString(`"`)
		}
		if block.Task {
			r.out.WriteString(` class="task-list-item"`)
		}
		r.out.WriteString(">")
		if block.Task {
			r.out.WriteString(`<input type="checkbox" disabled`)
			if block.Checked {
				r.out.WriteString(` checked`)
			}
			r.out.WriteString(` aria-hidden="true"> `)
		}
		r.renderInlines(block.Inlines)
		(*pos)++

		for *pos < len(blocks) {
			nextLevel := blocks[*pos].ListLevel
			if nextLevel <= level {
				break
			}
			r.renderListLevel(blocks, pos, nextLevel, blocks[*pos].Ordered)
		}

		r.out.WriteString("</li>\n")
		if ordered {
			if block.ListStart > 0 {
				expected = block.ListStart + 1
			} else {
				expected++
			}
		}
	}

	if ordered {
		r.out.WriteString("</ol>\n")
	} else {
		r.out.WriteString("</ul>\n")
	}
}

func (r *renderer) renderTable(block document.Block) {
	if len(block.Rows) == 0 {
		return
	}

	r.out.WriteString("<table>\n<thead>\n<tr>")
	for i, cell := range block.Rows[0].Cells {
		r.out.WriteString("<th")
		r.writeAlignment(block.Alignments, i)
		r.out.WriteString(">")
		r.renderInlines(cell.Inlines)
		r.out.WriteString("</th>")
	}
	r.out.WriteString("</tr>\n</thead>\n")

	if len(block.Rows) > 1 {
		r.out.WriteString("<tbody>\n")
		for _, row := range block.Rows[1:] {
			r.out.WriteString("<tr>")
			for i, cell := range row.Cells {
				r.out.WriteString("<td")
				r.writeAlignment(block.Alignments, i)
				r.out.WriteString(">")
				r.renderInlines(cell.Inlines)
				r.out.WriteString("</td>")
			}
			r.out.WriteString("</tr>\n")
		}
		r.out.WriteString("</tbody>\n")
	}
	r.out.WriteString("</table>\n")
}

func (r *renderer) writeAlignment(alignments []document.Alignment, index int) {
	if index < 0 || index >= len(alignments) {
		return
	}
	alignment := alignments[index]
	if alignment != document.AlignCenter && alignment != document.AlignRight {
		return
	}
	r.out.WriteString(` style="text-align:`)
	r.out.WriteString(string(alignment))
	r.out.WriteString(`"`)
}

func (r *renderer) renderInlines(inlines []document.Inline) {
	for _, inline := range inlines {
		switch inline.Kind {
		case document.InlineHardBreak:
			r.out.WriteString("<br>\n")
		case document.InlineImage:
			r.renderImage(inline)
		case document.InlineText:
			r.renderTextInline(inline)
		}
	}
}

func (r *renderer) renderTextInline(inline document.Inline) {
	content := stdhtml.EscapeString(inline.Text)
	if inline.Code {
		content = "<code>" + content + "</code>"
	}
	if inline.Bold {
		content = "<strong>" + content + "</strong>"
	}
	if inline.Italic {
		content = "<em>" + content + "</em>"
	}
	if inline.Strike {
		content = "<del>" + content + "</del>"
	}
	if inline.Underline {
		content = "<u>" + content + "</u>"
	}
	if href := safeLinkURL(inline.URL); href != "" {
		content = `<a href="` + stdhtml.EscapeString(href) + `" target="_blank" rel="noopener noreferrer">` + content + "</a>"
	}
	r.out.WriteString(content)
}

func (r *renderer) renderImage(inline document.Inline) {
	source := strings.TrimSpace(inline.ImageSource)
	alt := strings.TrimSpace(inline.Alt)
	if alt == "" {
		alt = "image"
	}

	if remote := safeRemoteImageURL(source); remote != "" {
		r.writeImageTag(remote, alt)
		return
	}

	if source == "" {
		r.writeImageFallback(alt)
		return
	}

	path := source
	if !filepath.IsAbs(path) {
		if strings.TrimSpace(r.options.BaseDir) == "" {
			if !r.options.EmbedLocalImages {
				r.writeImageTag(filepath.ToSlash(path), alt)
				return
			}
			r.writeImageFallback(alt)
			return
		}
		path = filepath.Join(r.options.BaseDir, path)
	}
	path = filepath.Clean(path)

	if !r.options.EmbedLocalImages {
		r.writeImageTag(filepath.ToSlash(source), alt)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		r.writeImageFallback(alt)
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		r.writeImageFallback(alt)
		return
	}
	dataURL := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
	r.writeImageTag(dataURL, alt)
}

func (r *renderer) writeImageTag(source, alt string) {
	r.out.WriteString(`<img src="`)
	r.out.WriteString(stdhtml.EscapeString(source))
	r.out.WriteString(`" alt="`)
	r.out.WriteString(stdhtml.EscapeString(alt))
	r.out.WriteString(`">`)
}

func (r *renderer) writeImageFallback(alt string) {
	r.out.WriteString(`<span class="missing-image">[Image: `)
	r.out.WriteString(stdhtml.EscapeString(alt))
	r.out.WriteString(`]</span>`)
}

func (r *renderer) uniqueHeadingID(text string) string {
	base := slugify(text)
	count := r.headingIDs[base] + 1
	r.headingIDs[base] = count
	if count == 1 {
		return base
	}
	return base + "-" + strconv.Itoa(count)
}

func slugify(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	var out strings.Builder
	pendingDash := false

	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if pendingDash && out.Len() > 0 {
				out.WriteByte('-')
			}
			pendingDash = false
			out.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			pendingDash = out.Len() > 0
		}
	}

	value := strings.Trim(out.String(), "-")
	if value == "" {
		return "section"
	}
	return value
}

func plainText(inlines []document.Inline) string {
	var out strings.Builder
	for _, inline := range inlines {
		switch inline.Kind {
		case document.InlineText:
			out.WriteString(inline.Text)
		case document.InlineImage:
			out.WriteString(inline.Alt)
		case document.InlineHardBreak:
			out.WriteByte(' ')
		}
	}
	return strings.TrimSpace(out.String())
}

func safeLinkURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" {
		return raw
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto":
		return raw
	default:
		return ""
	}
}

func safeRemoteImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return raw
	default:
		return ""
	}
}
