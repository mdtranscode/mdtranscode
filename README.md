# MDTranscode

MDTranscode is a Windows-first open-source utility for turning Markdown into professional documents.

Current capabilities include:

- Markdown -> DOCX
- semantic HTML rendering/preview
- Windows desktop application
- `.md` / `.markdown` Open With integration
- Explorer `Convert to Word with MDTranscode`
- non-overwriting file conversion
- optional foreground Folder Watcher for automatic Markdown-to-DOCX conversion

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

[Get MDTranscode from the Microsoft Store](https://apps.microsoft.com/detail/9MXCQXBPS8K1)

The Microsoft Store offers signed Windows installation and automatic updates. During certification, Store updates may lag behind the latest public source. Folder monitoring in the 1.0.5.0 source requires the app window to remain open; Windows startup and system-tray background operation are planned for a separate future iteration.
