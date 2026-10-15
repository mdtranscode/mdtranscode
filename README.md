# MDTranscode

MDTranscode is an open-source, Windows-first desktop application for converting Markdown files into professional, editable documents. Work locally on your PC using the desktop interface, Windows Explorer integration, or the command-line interface.

## Features

- Convert Markdown (`.md` and `.markdown`) to editable Microsoft Word (`.docx`) documents, including headings, tables, lists, links, images, and code.
- Export paginated PDF documents locally on Windows.
- Preview Markdown as semantic HTML, inspect the document outline, and search document content.
- Produce Word documents using standard, native/minimal, or supported custom `.dotx` styles, with options for branding, covers, headers and footers, pagination, and table of contents.
- Convert using Windows Explorer context-menu integration, with numbered filenames to avoid silently overwriting existing output.
- Use the folder watcher for supported unattended conversion workflows.
- Convert from the CLI with the same shared Go conversion engine as the Windows app.

Conversions run locally. Referenced remote images may require network access. Supported custom Word templates use a validated subset of `.dotx` styles/themes; arbitrary template document contents and macros are not imported. Some generated Word tables of contents may need updating in Word.

## Install

Install the Windows application through the [Microsoft Store](https://apps.microsoft.com/detail/9MXCQXBPS8K1), or build from the [public source repository](https://github.com/mdtranscode/mdtranscode). The Microsoft Store handles installation and updates; the public source includes the complete buildable application without a separate feature-gated edition.

## How it works

```text
Markdown -> parser -> semantic document -> DOCX / HTML / PDF renderer
```

The Windows app, Explorer integration, and CLI share the same conversion core. Renderers consume the parsed semantic document rather than reparsing Markdown independently.

## Source layout

- `core-src/` — Go Markdown parser, semantic document model, renderers, and CLI.
- `windows/` — Windows desktop application and installer inputs.

## Build and test

Core:

```powershell
cd core-src
go test ./...
go vet ./...
```

Windows development requires Go and Wails v2.15.0; NSIS is required for the classic installer.

```powershell
cd windows
go test ./...
go vet ./...
wails build -clean
```

## Support and links

- Website: https://mdtransco.de
- Support: https://mdtransco.de/support
- Public source and issues: https://github.com/mdtranscode/mdtranscode
- Microsoft Store: https://apps.microsoft.com/detail/9MXCQXBPS8K1

## License

MDTranscode source is licensed under the Mozilla Public License 2.0. See [`LICENSE`](LICENSE) and [`NOTICE.md`](NOTICE.md).
