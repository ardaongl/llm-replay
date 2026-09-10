# Contributing to LLM Replay

Thanks for helping make model migrations safer and easier to verify.

## Development setup

You need Go 1.22 or newer. Fork and clone the repository, then run:

```bash
make test
make lint
make smoke
```

Without Make, use `go test -race ./...`, `golangci-lint run`, and
`go run ./scripts/smoke-test`.

## Making a change

1. Open an issue for substantial behavior or data-format changes.
2. Keep provider-specific logic inside `internal/provider/<name>`.
3. Preserve the normalized domain contract and append-only run artifacts.
4. Add focused unit tests and an integration test when network behavior changes.
5. Never commit API keys, captured customer data, or generated run artifacts.

Run `gofmt` on changed Go files. Pull requests must pass formatting, vet, race
tests, lint, the generated-fixture check, the smoke test, and a clean build.

## Example dataset

The checked-in support fixture is synthetic and deterministic. Regenerate it
after changing its source scenarios:

```bash
make examples
git diff -- examples/datasets/support.jsonl
```

Do not add real customer prompts or identifying information to examples.

## Releases

Maintainers create a semantic version tag such as `v0.1.0`. The release
workflow tests the tag and uses GoReleaser to publish checksummed Linux, macOS,
and Windows archives. Test the packaging locally with `make snapshot` first.

## Reporting security issues

Do not open a public issue for a suspected secret leak or security
vulnerability. Contact the maintainer privately through the security contact
listed on the repository profile.

