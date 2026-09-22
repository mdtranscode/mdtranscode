package markdown

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mdtranscode/mdtranscode/core-src/document"
)

var (
	reHeading    = regexp.MustCompile(`^ {0,3}(#{1,6})[\t ]+(.*?)[\t ]*#*[\t ]*$`)
	reSetext     = regexp.MustCompile(`^ {0,3}(=+|-+)[\t ]*$`)
	reList       = regexp.MustCompile(`^( *)([-+*]|([0-9]{1,9})[.)])[\t ]+(.*)$`)
	reFence      = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	reRule       = regexp.MustCompile(`^ {0,3}((\*[\t ]*){3,}|(-[\t ]*){3,}|(_[\t ]*){3,})$`)
	reQuote      = regexp.MustCompile(`^ {0,3}>[\t ]?(.*)$`)
	rePreOpen    = regexp.MustCompile(`(?i)^\s*<pre(?:\s[^>]*)?>`)
	rePreClose   = regexp.MustCompile(`(?i)</pre>\s*$`)
	reCodeOpen   = regexp.MustCompile(`(?i)^\s*<code(?:\s[^>]*)?>`)
	reCodeClose  = regexp.MustCompile(`(?i)</code>\s*$`)
	reTableSep   = regexp.MustCompile(`^:?-{3,}:?$`)
	reTask       = regexp.MustCompile(`^\[([ xX])\][\t ]+(.*)$`)
	reRefDef     = regexp.MustCompile(`^ {0,3}\[([^\]]+)\]:[\t ]*(?:<([^>]+)>|(\S+))(?:[\t ]+.*)?$`)
	reHTMLAnchor = regexp.MustCompile(`(?i)^<a\s+[^>]*href=["\']([^"\']+)["\'][^>]*>`)
	reHTMLImage  = regexp.MustCompile(`(?i)^<img\s+[^>]*src=["\']([^"\']+)["\'][^>]*>`)
	reHTMLAlt    = regexp.MustCompile(`(?i)\balt=["\']([^"\']*)["\']`)
)

// Parser owns parse state such as reference-style link definitions. Parser
// instances are independent and safe to use concurrently as separate values.
type Parser struct {
	refs map[string]string
}

// Parse converts Markdown source into the target-neutral document model.
func Parse(source string) *document.Document {
	p := &Parser{refs: make(map[string]string)}
	return p.Parse(source)
}

// Parse converts Markdown source into the target-neutral document model.
func (p *Parser) Parse(source string) *document.Document {
	text := strings.ReplaceAll(source, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 {
		lines[0] = strings.TrimPrefix(lines[0], "\ufeff")
	}

	p.refs = make(map[string]string)
	p.collectReferences(lines)

	return &document.Document{Blocks: p.parseLines(lines, 0)}
}

func (p *Parser) collectReferences(lines []string) {
	inFence := false
	var fence byte

	for _, line := range lines {
		if m := reFence.FindStringSubmatch(line); m != nil {
			ch := m[1][0]
			if !inFence {
				inFence = true
				fence = ch
			} else if ch == fence {
				inFence = false
			}
			continue
		}
		if inFence || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if m := reRefDef.FindStringSubmatch(line); m != nil {
			dest := m[2]
			if dest == "" {
				dest = m[3]
			}
			p.refs[normalizeRefLabel(m[1])] = html.UnescapeString(dest)
		}
	}
}

func (p *Parser) parseLines(lines []string, quoteLevel int) []document.Block {
	var out []document.Block
	listGroupCounter := 0
	activeOrderedGroup := 0
	activeOrderedLevel := -1
	activeOrderedQuote := -1

	resetList := func() {
		activeOrderedGroup = 0
		activeOrderedLevel = -1
		activeOrderedQuote = -1
	}

	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			resetList()
			i++
			continue
		}
		if reRefDef.MatchString(line) {
			resetList()
			i++
			continue
		}

		if m := reQuote.FindStringSubmatch(line); m != nil {
			var q []string
			for i < len(lines) {
				if qm := reQuote.FindStringSubmatch(lines[i]); qm != nil {
					q = append(q, qm[1])
					i++
				} else if strings.TrimSpace(lines[i]) == "" {
					q = append(q, "")
					i++
				} else {
					break
				}
			}
			out = append(out, p.parseLines(q, quoteLevel+1)...)
			resetList()
			continue
		}

		if m := reFence.FindStringSubmatch(line); m != nil {
			marker := m[1]
			ch := marker[0]
			n := len(marker)
			var body []string
			i++
			for i < len(lines) {
				trimmed := strings.TrimLeft(lines[i], " ")
				lead := len(lines[i]) - len(trimmed)
				if lead <= 3 && len(trimmed) >= n {
					run := 0
					for run < len(trimmed) && trimmed[run] == ch {
						run++
					}
					if run >= n && strings.TrimSpace(trimmed[run:]) == "" {
						i++
						break
					}
				}
				body = append(body, lines[i])
				i++
			}
			out = append(out, document.Block{Kind: document.BlockCode, Text: strings.Join(body, "\n"), QuoteLevel: quoteLevel})
			resetList()
			continue
		}

		if rePreOpen.MatchString(line) {
			var body []string
			first := rePreOpen.ReplaceAllString(line, "")
			closed := rePreClose.MatchString(first)
			first = rePreClose.ReplaceAllString(first, "")
			first = reCodeOpen.ReplaceAllString(first, "")
			first = reCodeClose.ReplaceAllString(first, "")
			if first != "" {
				body = append(body, first)
			}
			i++
			for i < len(lines) && !closed {
				cur := lines[i]
				if rePreClose.MatchString(cur) {
					cur = rePreClose.ReplaceAllString(cur, "")
					closed = true
				}
				cur = reCodeOpen.ReplaceAllString(cur, "")
				cur = reCodeClose.ReplaceAllString(cur, "")
				body = append(body, cur)
				i++
			}
			out = append(out, document.Block{Kind: document.BlockCode, Text: html.UnescapeString(strings.Join(body, "\n")), QuoteLevel: quoteLevel})
			resetList()
			continue
		}

		if m := reHeading.FindStringSubmatch(line); m != nil {
			out = append(out, document.Block{
				Kind:       document.BlockHeading,
				Level:      len(m[1]),
				Inlines:    p.parseInline(m[2], inlineStyle{}),
				QuoteLevel: quoteLevel,
			})
			i++
			resetList()
			continue
		}

		if i+1 < len(lines) && strings.TrimSpace(line) != "" {
			if m := reSetext.FindStringSubmatch(lines[i+1]); m != nil && !reRule.MatchString(lines[i+1]) {
				level := 2
				if strings.HasPrefix(strings.TrimSpace(m[1]), "=") {
					level = 1
				}
				out = append(out, document.Block{
					Kind:       document.BlockHeading,
					Level:      level,
					Inlines:    p.parseInline(strings.TrimSpace(line), inlineStyle{}),
					QuoteLevel: quoteLevel,
				})
				i += 2
				resetList()
				continue
			}
		}

		if i+1 < len(lines) && looksLikeTableHeader(line, lines[i+1]) {
			header := splitTableRow(line)
			sep := splitTableRow(lines[i+1])
			alignments := make([]document.Alignment, len(sep))
			for c, s := range sep {
				s = strings.TrimSpace(s)
				left := strings.HasPrefix(s, ":")
				right := strings.HasSuffix(s, ":")
				switch {
				case left && right:
					alignments[c] = document.AlignCenter
				case right:
					alignments[c] = document.AlignRight
				default:
					alignments[c] = document.AlignLeft
				}
			}

			rows := [][]string{header}
			i += 2
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && strings.Contains(lines[i], "|") {
				row := splitTableRow(lines[i])
				for len(row) < len(header) {
					row = append(row, "")
				}
				if len(row) > len(header) {
					row = row[:len(header)]
				}
				rows = append(rows, row)
				i++
			}

			parsedRows := make([]document.TableRow, 0, len(rows))
			for _, row := range rows {
				parsedRow := document.TableRow{Cells: make([]document.TableCell, 0, len(row))}
				for _, cell := range row {
					parsedRow.Cells = append(parsedRow.Cells, document.TableCell{Inlines: p.parseInline(cell, inlineStyle{})})
				}
				parsedRows = append(parsedRows, parsedRow)
			}

			out = append(out, document.Block{
				Kind:       document.BlockTable,
				Rows:       parsedRows,
				Alignments: alignments,
				QuoteLevel: quoteLevel,
			})
			resetList()
			continue
		}

		if reRule.MatchString(line) {
			out = append(out, document.Block{Kind: document.BlockRule, QuoteLevel: quoteLevel})
			i++
			resetList()
			continue
		}

		if m := reList.FindStringSubmatch(line); m != nil {
			spaces := len(m[1])
			level := spaces / 2
			ordered := m[3] != ""
			start := 1
			if ordered {
				start, _ = strconv.Atoi(m[3])
			}
			content := m[4]
			task := false
			checked := false
			if tm := reTask.FindStringSubmatch(content); tm != nil {
				task = true
				checked = strings.EqualFold(tm[1], "x")
				content = tm[2]
			}

			group := 0
			if ordered {
				if activeOrderedGroup == 0 || activeOrderedLevel != level || activeOrderedQuote != quoteLevel {
					listGroupCounter++
					activeOrderedGroup = listGroupCounter
					activeOrderedLevel = level
					activeOrderedQuote = quoteLevel
				}
				group = activeOrderedGroup
			} else {
				activeOrderedGroup = 0
			}

			block := document.Block{
				Kind:       document.BlockListItem,
				Ordered:    ordered,
				ListLevel:  level,
				ListStart:  start,
				ListGroup:  group,
				QuoteLevel: quoteLevel,
				Task:       task,
				Checked:    checked,
			}
			i++
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "" {
					break
				}
				if reList.MatchString(lines[i]) || reQuote.MatchString(lines[i]) || reFence.MatchString(lines[i]) {
					break
				}
				indent := leadingSpaces(lines[i])
				if indent >= spaces+2 {
					content += "\n" + strings.TrimSpace(lines[i])
					i++
				} else {
					break
				}
			}
			block.Inlines = p.parseInline(content, inlineStyle{})
			out = append(out, block)
			continue
		}

		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			var body []string
			for i < len(lines) {
				cur := lines[i]
				if strings.HasPrefix(cur, "    ") {
					body = append(body, cur[4:])
					i++
					continue
				}
				if strings.HasPrefix(cur, "\t") {
					body = append(body, cur[1:])
					i++
					continue
				}
				if strings.TrimSpace(cur) == "" {
					body = append(body, "")
					i++
					continue
				}
				break
			}
			for len(body) > 0 && body[len(body)-1] == "" {
				body = body[:len(body)-1]
			}
			out = append(out, document.Block{Kind: document.BlockCode, Text: strings.Join(body, "\n"), QuoteLevel: quoteLevel})
			resetList()
			continue
		}

		var paragraph []string
		for i < len(lines) {
			cur := lines[i]
			if strings.TrimSpace(cur) == "" {
				break
			}
			if len(paragraph) > 0 && isBlockStart(lines, i) {
				break
			}
			paragraph = append(paragraph, cur)
			i++
			if i < len(lines) && len(paragraph) == 1 && looksLikeTableHeader(paragraph[0], lines[i]) {
				break
			}
		}
		if len(paragraph) > 0 {
			out = append(out, document.Block{
				Kind:       document.BlockParagraph,
				Inlines:    p.parseInline(strings.Join(paragraph, "\n"), inlineStyle{}),
				QuoteLevel: quoteLevel,
			})
		} else {
			i++
		}
		resetList()
	}

	return out
}

func isBlockStart(lines []string, i int) bool {
	if i < 0 || i >= len(lines) {
		return false
	}
	s := lines[i]
	if reQuote.MatchString(s) || reFence.MatchString(s) || reHeading.MatchString(s) || reRule.MatchString(s) || reList.MatchString(s) || rePreOpen.MatchString(s) || strings.HasPrefix(s, "    ") || strings.HasPrefix(s, "\t") {
		return true
	}
	return i+1 < len(lines) && looksLikeTableHeader(s, lines[i+1])
}

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func looksLikeTableHeader(header, sep string) bool {
	if !strings.Contains(header, "|") || !strings.Contains(sep, "-") {
		return false
	}
	cells := splitTableRow(sep)
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		c = strings.ReplaceAll(strings.TrimSpace(c), " ", "")
		if !reTableSep.MatchString(c) {
			return false
		}
	}
	return len(splitTableRow(header)) == len(cells)
}

func splitTableRow(s string) []string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "|") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, "\\|") {
		s = s[:len(s)-1]
	}

	var cells []string
	var b strings.Builder
	escaped := false
	codeTicks := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			if i+1 < len(s) && s[i+1] == '|' {
				escaped = true
				continue
			}
			escaped = true
			b.WriteByte(c)
			continue
		}
		if c == '`' {
			codeTicks ^= 1
			b.WriteByte(c)
			continue
		}
		if c == '|' && codeTicks == 0 {
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	cells = append(cells, strings.TrimSpace(b.String()))
	return cells
}

type inlineStyle struct {
	bold      bool
	italic    bool
	strike    bool
	code      bool
	underline bool
	url       string
}

func (p *Parser) parseInline(s string, style inlineStyle) []document.Inline {
	var out []document.Inline

	appendText := func(text string, st inlineStyle) {
		if text == "" {
			return
		}
		out = append(out, document.Inline{
			Kind:      document.InlineText,
			Text:      html.UnescapeString(text),
			Bold:      st.bold,
			Italic:    st.italic,
			Strike:    st.strike,
			Code:      st.code,
			Underline: st.underline,
			URL:       st.url,
		})
	}

	for len(s) > 0 {
		if s[0] == '\\' && len(s) > 1 && strings.ContainsRune(`\\`+"`*_{}[]()#+-.!|>~", rune(s[1])) {
			appendText(s[1:2], style)
			s = s[2:]
			continue
		}

		if s[0] == '\n' {
			hard := false
			if len(out) > 0 && out[len(out)-1].Kind == document.InlineText {
				text := out[len(out)-1].Text
				if strings.HasSuffix(text, "  ") {
					out[len(out)-1].Text = strings.TrimSuffix(text, "  ")
					hard = true
				}
				if !hard && strings.HasSuffix(text, "\\") {
					out[len(out)-1].Text = strings.TrimSuffix(text, "\\")
					hard = true
				}
			}
			if hard {
				out = append(out, document.Inline{Kind: document.InlineHardBreak})
			} else {
				appendText(" ", style)
			}
			s = s[1:]
			continue
		}

		lower := strings.ToLower(s)
		if strings.HasPrefix(lower, "<br>") || strings.HasPrefix(lower, "<br/>") || strings.HasPrefix(lower, "<br />") {
			n := 4
			if strings.HasPrefix(lower, "<br/>") {
				n = 5
			}
			if strings.HasPrefix(lower, "<br />") {
				n = 6
			}
			out = append(out, document.Inline{Kind: document.InlineHardBreak})
			s = s[n:]
			continue
		}

		if handled, consumed, inlines := p.parseInlineHTML(s, style); handled {
			out = append(out, inlines...)
			s = s[consumed:]
			continue
		}

		if s[0] == '`' {
			n := 1
			for n < len(s) && s[n] == '`' {
				n++
			}
			delim := s[:n]
			if j := strings.Index(s[n:], delim); j >= 0 {
				raw := s[n : n+j]
				raw = strings.ReplaceAll(raw, "\n", " ")
				if len(raw) >= 2 && strings.HasPrefix(raw, " ") && strings.HasSuffix(raw, " ") && strings.TrimSpace(raw) != "" {
					raw = raw[1 : len(raw)-1]
				}
				codeStyle := style
				codeStyle.code = true
				appendText(raw, codeStyle)
				s = s[n+j+n:]
				continue
			}
		}

		if strings.HasPrefix(s, "![") {
			if alt, dest, consumed, ok := parseLinkLike(s, true); ok {
				out = append(out, document.Inline{Kind: document.InlineImage, ImageSource: dest, Alt: alt})
				s = s[consumed:]
				continue
			}
		}

		if strings.HasPrefix(s, "![") {
			if label, dest, consumed, ok := p.parseReferenceLike(s, true); ok {
				out = append(out, document.Inline{Kind: document.InlineImage, ImageSource: dest, Alt: label})
				s = s[consumed:]
				continue
			}
		}

		if strings.HasPrefix(s, "[") {
			if label, dest, consumed, ok := p.parseReferenceLike(s, false); ok {
				linkStyle := style
				linkStyle.url = dest
				linkStyle.underline = true
				out = append(out, p.parseInline(label, linkStyle)...)
				s = s[consumed:]
				continue
			}
		}

		if strings.HasPrefix(s, "[") {
			if label, dest, consumed, ok := parseLinkLike(s, false); ok {
				linkStyle := style
				linkStyle.url = dest
				linkStyle.underline = true
				out = append(out, p.parseInline(label, linkStyle)...)
				s = s[consumed:]
				continue
			}
		}

		if s[0] == '<' {
			if j := strings.IndexByte(s, '>'); j > 1 {
				inside := s[1:j]
				if strings.HasPrefix(inside, "http://") || strings.HasPrefix(inside, "https://") || strings.HasPrefix(inside, "mailto:") {
					linkStyle := style
					linkStyle.url = inside
					linkStyle.underline = true
					appendText(strings.TrimPrefix(inside, "mailto:"), linkStyle)
					s = s[j+1:]
					continue
				}
				if strings.Contains(inside, "@") && !strings.ContainsAny(inside, " <>\t") {
					linkStyle := style
					linkStyle.url = "mailto:" + inside
					linkStyle.underline = true
					appendText(inside, linkStyle)
					s = s[j+1:]
					continue
				}
			}
		}

		if strings.HasPrefix(s, "***") || strings.HasPrefix(s, "___") {
			delim := s[:3]
			if j := findClosing(s[3:], delim); j >= 0 {
				nested := style
				nested.bold = true
				nested.italic = true
				out = append(out, p.parseInline(s[3:3+j], nested)...)
				s = s[3+j+3:]
				continue
			}
		}

		if strings.HasPrefix(s, "**") || strings.HasPrefix(s, "__") {
			delim := s[:2]
			if j := findClosing(s[2:], delim); j >= 0 {
				nested := style
				nested.bold = true
				out = append(out, p.parseInline(s[2:2+j], nested)...)
				s = s[2+j+2:]
				continue
			}
		}

		if strings.HasPrefix(s, "~~") {
			if j := findClosing(s[2:], "~~"); j >= 0 {
				nested := style
				nested.strike = true
				out = append(out, p.parseInline(s[2:2+j], nested)...)
				s = s[2+j+2:]
				continue
			}
		}

		if s[0] == '*' || s[0] == '_' {
			delim := s[:1]
			if s[0] == '_' && len(s) > 1 && isAlphaNumBeforeAfter(s, 0) {
				// Literal underscore inside a word.
			} else if j := findClosing(s[1:], delim); j >= 0 {
				nested := style
				nested.italic = true
				out = append(out, p.parseInline(s[1:1+j], nested)...)
				s = s[1+j+1:]
				continue
			}
		}

		if strings.HasPrefix(s, "  \n") {
			out = append(out, document.Inline{Kind: document.InlineHardBreak})
			s = s[3:]
			continue
		}
		if strings.HasPrefix(s, "\\\n") {
			out = append(out, document.Inline{Kind: document.InlineHardBreak})
			s = s[2:]
			continue
		}

		n := 1
		for n < len(s) && !strings.ContainsRune("\\\n`![<*_~", rune(s[n])) {
			n++
		}
		appendText(s[:n], style)
		s = s[n:]
	}

	return mergeInlines(out)
}

func isAlphaNumBeforeAfter(s string, i int) bool {
	var before, after rune
	if i > 0 {
		before, _ = utf8.DecodeLastRuneInString(s[:i])
	}
	if i+1 < len(s) {
		after, _ = utf8.DecodeRuneInString(s[i+1:])
	}
	return (unicode.IsLetter(before) || unicode.IsDigit(before)) && (unicode.IsLetter(after) || unicode.IsDigit(after))
}

func findClosing(s, delim string) int {
	for off := 0; ; {
		j := strings.Index(s[off:], delim)
		if j < 0 {
			return -1
		}
		j += off
		if j == 0 || s[j-1] != '\\' {
			return j
		}
		off = j + len(delim)
		if off >= len(s) {
			return -1
		}
	}
}

func parseLinkLike(s string, image bool) (label, dest string, consumed int, ok bool) {
	start := 1
	if image {
		start = 2
	}
	if len(s) <= start || s[start-1] != '[' {
		return
	}

	end := -1
	escaped := false
	for i := start; i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		if s[i] == ']' {
			end = i
			break
		}
	}
	if end < 0 || end+1 >= len(s) || s[end+1] != '(' {
		return
	}

	position := end + 2
	depth := 1
	inAngle := false
	escaped = false
	for i := position; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '<' {
			inAngle = true
		}
		if c == '>' {
			inAngle = false
		}
		if !inAngle {
			if c == '(' {
				depth++
			}
			if c == ')' {
				depth--
				if depth == 0 {
					raw := strings.TrimSpace(s[position:i])
					label = s[start:end]
					dest = parseDestination(raw)
					consumed = i + 1
					ok = dest != ""
					return
				}
			}
		}
	}
	return
}

func (p *Parser) parseInlineHTML(s string, style inlineStyle) (bool, int, []document.Inline) {
	lower := strings.ToLower(s)
	type wrapper struct {
		open    string
		close   string
		apply   func(*inlineStyle)
		literal bool
	}
	wrappers := []wrapper{
		{"<strong>", "</strong>", func(x *inlineStyle) { x.bold = true }, false},
		{"<b>", "</b>", func(x *inlineStyle) { x.bold = true }, false},
		{"<em>", "</em>", func(x *inlineStyle) { x.italic = true }, false},
		{"<i>", "</i>", func(x *inlineStyle) { x.italic = true }, false},
		{"<del>", "</del>", func(x *inlineStyle) { x.strike = true }, false},
		{"<s>", "</s>", func(x *inlineStyle) { x.strike = true }, false},
		{"<strike>", "</strike>", func(x *inlineStyle) { x.strike = true }, false},
		{"<u>", "</u>", func(x *inlineStyle) { x.underline = true }, false},
		{"<code>", "</code>", func(x *inlineStyle) { x.code = true }, true},
	}

	for _, wrapper := range wrappers {
		if strings.HasPrefix(lower, wrapper.open) {
			j := strings.Index(lower[len(wrapper.open):], wrapper.close)
			if j < 0 {
				continue
			}
			j += len(wrapper.open)
			inner := s[len(wrapper.open):j]
			nested := style
			wrapper.apply(&nested)
			var out []document.Inline
			if wrapper.literal {
				out = []document.Inline{{
					Kind:      document.InlineText,
					Text:      html.UnescapeString(inner),
					Bold:      nested.bold,
					Italic:    nested.italic,
					Strike:    nested.strike,
					Code:      nested.code,
					Underline: nested.underline,
					URL:       nested.url,
				}}
			} else {
				out = p.parseInline(inner, nested)
			}
			return true, j + len(wrapper.close), out
		}
	}

	if m := reHTMLAnchor.FindStringSubmatch(s); m != nil {
		openLen := len(m[0])
		j := strings.Index(strings.ToLower(s[openLen:]), "</a>")
		if j >= 0 {
			j += openLen
			nested := style
			nested.url = html.UnescapeString(m[1])
			nested.underline = true
			return true, j + 4, p.parseInline(s[openLen:j], nested)
		}
	}

	if m := reHTMLImage.FindStringSubmatch(s); m != nil {
		alt := ""
		if am := reHTMLAlt.FindStringSubmatch(m[0]); am != nil {
			alt = html.UnescapeString(am[1])
		}
		return true, len(m[0]), []document.Inline{{
			Kind:        document.InlineImage,
			ImageSource: html.UnescapeString(m[1]),
			Alt:         alt,
		}}
	}

	return false, 0, nil
}

func normalizeRefLabel(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func (p *Parser) parseReferenceLike(s string, image bool) (label, dest string, consumed int, ok bool) {
	start := 1
	if image {
		start = 2
	}
	if len(s) <= start || s[start-1] != '[' {
		return
	}

	end := strings.Index(s[start:], "]")
	if end < 0 {
		return
	}
	end += start
	label = s[start:end]
	after := end + 1
	ref := label
	if after < len(s) && s[after] == '[' {
		j := strings.Index(s[after+1:], "]")
		if j < 0 {
			return
		}
		j += after + 1
		if j > after+1 {
			ref = s[after+1 : j]
		}
		consumed = j + 1
	} else {
		consumed = after
	}

	dest, ok = p.refs[normalizeRefLabel(ref)]
	return
}

func parseDestination(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "<") {
		if j := strings.Index(raw, ">"); j > 0 {
			return html.UnescapeString(raw[1:j])
		}
	}

	var b strings.Builder
	escaped := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if unicode.IsSpace(rune(c)) {
			break
		}
		b.WriteByte(c)
	}
	return html.UnescapeString(b.String())
}

func mergeInlines(in []document.Inline) []document.Inline {
	var out []document.Inline
	for _, inline := range in {
		if len(out) > 0 && inline.Kind == document.InlineText && out[len(out)-1].Kind == document.InlineText &&
			inline.Bold == out[len(out)-1].Bold && inline.Italic == out[len(out)-1].Italic &&
			inline.Strike == out[len(out)-1].Strike && inline.Code == out[len(out)-1].Code &&
			inline.Underline == out[len(out)-1].Underline && inline.URL == out[len(out)-1].URL {
			out[len(out)-1].Text += inline.Text
		} else {
			out = append(out, inline)
		}
	}
	return out
}
