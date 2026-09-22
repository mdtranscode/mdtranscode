package document

// Document is the target-neutral representation produced by the Markdown parser.
// Renderers consume this model; they must not reparse Markdown source.
type Document struct {
	Blocks []Block
}

// BlockKind identifies a block-level document node.
type BlockKind int

const (
	BlockParagraph BlockKind = iota
	BlockHeading
	BlockCode
	BlockListItem
	BlockTable
	BlockRule
)

// Alignment is a target-neutral table-cell alignment.
type Alignment string

const (
	AlignLeft   Alignment = "left"
	AlignCenter Alignment = "center"
	AlignRight  Alignment = "right"
)

// Block represents one block-level node. Fields that are not meaningful for
// the block Kind are left at their zero value.
type Block struct {
	Kind BlockKind

	// Paragraph/heading/list content.
	Inlines []Inline

	// Heading level, 1..6.
	Level int

	// Code block content.
	Text string

	// Number of enclosing blockquotes.
	QuoteLevel int

	// List metadata. Keeping list items as blocks mirrors prototype behavior
	// while remaining target-neutral; renderers can later normalize/group them.
	Ordered   bool
	ListLevel int
	ListStart int
	ListGroup int
	Task      bool
	Checked   bool

	// Table data.
	Rows       []TableRow
	Alignments []Alignment
}

// TableRow is one table row.
type TableRow struct {
	Cells []TableCell
}

// TableCell is one table cell, represented as parsed inline content.
type TableCell struct {
	Inlines []Inline
}

// InlineKind identifies an inline node.
type InlineKind int

const (
	InlineText InlineKind = iota
	InlineImage
	InlineHardBreak
)

// Inline is a target-neutral inline node.
type Inline struct {
	Kind InlineKind

	Text string

	Bold      bool
	Italic    bool
	Strike    bool
	Code      bool
	Underline bool

	URL string

	ImageSource string
	Alt         string
}
