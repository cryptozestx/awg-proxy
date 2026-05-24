# Contributing to awg-proxy

Thank you for considering a contribution! Here is everything you need to know.

## Getting started

1. **Fork** the repository and clone your fork.
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes, add tests where appropriate.
4. Run `go vet ./...` and `go test ./...` to verify nothing is broken.
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/) style:
   - `feat: add UDP proxy support`
   - `fix: correct UAPI key ordering for replace_peers`
   - `docs: improve example.conf annotations`
6. Push your branch and **open a Pull Request** against `main`.

## Code style

- Run `gofmt -w .` before committing.
- Keep functions focused and small.
- Comment exported types and functions.

## Reporting bugs

Open an [issue](../../issues/new?template=bug_report.md) and fill in the template. Please include:
- Your OS and Go version.
- The exact command you ran (with sensitive values redacted).
- The full error output.

## Suggesting features

Open an [issue](../../issues/new?template=feature_request.md) with the "enhancement" label.

## Security vulnerabilities

**Do not open a public issue.** Instead, please report security issues privately via GitHub's [Security Advisory](../../security/advisories/new) feature.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
