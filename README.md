# MDTranscode

## What's Immediately Next on the Roadmap

- **v1.0.8.0 Release:** finish MSIX/source verification, GitHub publication and Store submission.

## What's on the Roadmap After That

- **Immediate after release — Public website overhaul:** compare HostGator to Git, version deployment/rollback, and improve feature pages, performance and SEO.
- **Milestone 8 — Combined Client Export:** produce DOCX and PDF together.
- **Milestone 9 — Batch Conversion:** process files and folders in batches.
- **Milestone 10 — Advanced Automation:** conversion profiles, CLI/CI automation, and deployment.
- **Milestone 11 — Advanced Markdown:** richer Markdown and publishing directives.
- **Milestone 12 — Website / Community Expansion:** documentation, examples, templates, and support resources.

## What's Been Built So Far

- **Core:** shared Markdown parser, semantic document model, DOCX renderer, CLI, and semantic HTML rendering.
- **Windows:** Wails desktop app, preview/outline, file handling, settings, tray, startup preferences, and DOCX Folder Watcher.
- **Explorer:** `.md` / `.markdown` Open With integration plus non-overwriting Convert to Word and Convert to PDF commands.
- **PDF:** paginated Letter/A4 export with page settings, local images, headers/footers, page numbers, and bookmarks (Iteration 3 accepted; GitHub v1.0.7.0 published).
- **Word templates (Iteration 4 accepted):** Standard, Native/Minimal and validated custom `.dotx` styles/themes; branded cover, logo, headers/footers, page numbers, TOC, CLI, Settings and Explorer; source version 1.0.8.0 with Store delivery tracked separately.

## About MDTranscode

MDTranscode is a Windows-first open-source utility for turning Markdown into professional documents.

Available capabilities include:

- Markdown to DOCX
- Markdown to paginated PDF
- Semantic HTML rendering/preview
- Windows desktop conversion and Explorer integration
- Locally rendered PDF output using Microsoft Edge on Windows

The conversion pipeline is shared across the CLI and Windows application:

    Markdown -> parser -> document.Document -> renderer

Renderers do not reparse Markdown. Windows uses the same Core conversion code as the CLI.

## Source Layout

    core-src/    reusable Go conversion engine and CLI
    windows/     Windows/Wails application and installer inputs

Buildable open-source application source is available in the public MDTranscode repository. The project website and Microsoft Store releases are maintained separately.

## Build and Test

Core:

    cd core-src
    go test ./...
    go vet ./...

Windows development requires Go and Wails v2.15.0; NSIS is required for the classic installer.

    cd windows
    go test ./...
    go vet ./...
    wails build -clean

## License

MDTranscode source is licensed under the Mozilla Public License 2.0. See `LICENSE` and `NOTICE.md`.

Project website: https://mdtransco.de

## Install from Microsoft Store

Store listing: https://apps.microsoft.com/detail/9MXCQXBPS8K1

The Store build provides the same functional product as the buildable public source, with Microsoft Store-managed installation and updates.
