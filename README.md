# MDTranscode

MDTranscode is a Windows-first open-source utility for turning Markdown into professional documents.

Current capabilities include:

- Markdown -> DOCX
- Markdown -> paginated PDF (Letter/A4, orientation, margins, optional header/footer, page numbers and bookmarks)
- semantic HTML rendering/preview
- Windows desktop application
- `.md` / `.markdown` Open With integration
- Explorer `Convert to Word with MDTranscode` and `Convert to PDF with MDTranscode`
- non-overwriting Explorer file conversion
- locally rendered PDF output using Microsoft Edge on Windows

The conversion pipeline is intentionally shared across the CLI and Windows application:

    Markdown -> parser -> document.Document -> renderer

Renderers do not reparse Markdown. The Windows application consumes the same Core conversion code used by the CLI.

## Source Layout

    core-src/    reusable Go conversion engine and CLI
    windows/     Windows/Wails application and installer inputs

## Build

Core:

    cd core-src
    go test ./...
    go vet ./...

Windows development requires Go plus Wails v2.15.0. NSIS is required to build the classic installer.

    cd windows
    go test ./...
    go vet ./...
    wails build -clean

## License

MDTranscode source is licensed under the Mozilla Public License 2.0. See `LICENSE` and `NOTICE.md`.

Project website: https://mdtransco.de

## Install from Microsoft Store

The signed Windows package is available from the Microsoft Store:

    https://apps.microsoft.com/detail/9MXCQXBPS8K1

The Store package contains the same functional product as the public source and provides the simplest Windows install/update experience.
