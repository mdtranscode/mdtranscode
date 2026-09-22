# Contributing to MDTranscode

Thanks for helping improve MDTranscode.

## License

MDTranscode is licensed under MPL-2.0. By submitting a contribution, you agree that your contribution may be distributed under the repository's MPL-2.0 license and confirm that you have the right to submit it.

No separate CLA or DCO sign-off is currently required.

## Development Rules

Preserve the core architecture:

    Markdown -> parser -> document.Document -> renderer

Renderers must not reparse Markdown. Windows code may depend on Core; Core must not depend on Windows.

File conversion orchestration belongs in `core-src/transcode` so CLI, Windows export, and Explorer conversion share behavior.

Generated DOCX files must retain modern Word compatibility mode 15.

## Before a Pull Request

At minimum, run:

    cd core-src
    go test ./...
    go vet ./...

and:

    cd windows
    go test ./...
    go vet ./...

Windows integration changes should also be exercised through a local Wails/NSIS build before submission.

Do not commit credentials, signing keys, private account data, personal local paths, generated installers, or other local build artifacts.

## Scope

Small, focused changes are preferred. For large behavior or architecture changes, open an issue first so the design can be discussed before implementation.
