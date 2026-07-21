# Contributing to Pyxis

Thanks for your interest in improving Pyxis. This guide covers how to propose changes safely and consistently.

## Code of conduct

Be respectful in issues, discussions, and reviews. Assume good intent, keep feedback technical, and do not share secrets or cluster credentials in public tickets.

## License reminder

Pyxis is licensed under **Apache License 2.0 with Commons Clause**. Contributions are accepted under the same terms. In particular, Commons Clause restricts selling the software or offering a product/service whose value derives substantially from Pyxis functionality. See [`LICENSE`](./LICENSE).

## Development setup

Requirements:

- Go **1.26.5+** (see `go` directive in `go.mod`)
- A valid kubeconfig for integration testing against a cluster (optional for unit tests)
- `make` and Docker (optional, for Compose / release smoke checks)

```bash
git clone https://github.com/zrougamed/pyxis.git
cd pyxis
make test
make build
./bin/pyxis version
```

Useful targets:

| Target | Purpose |
|--------|---------|
| `make test` | Unit tests with race detector |
| `make lint` | golangci-lint |
| `make build` | Build local `bin/pyxis` |
| `make cross-build` | Multi-platform binaries |
| `make run` / `make run-web` | Local TUI / web UI |

## Branching and pull requests

1. Open an issue for non-trivial changes when practical.
2. Create a topic branch from `master` (`feature/…`, `fix/…`, `docs/…`).
3. Keep commits focused; prefer small reviewable diffs.
4. Ensure `make test` and `make lint` pass locally.
5. Open a PR with:
   - **Summary** of the change and why
   - **Test plan** (commands / UI paths exercised)
   - Screenshots for web UI changes when relevant

## Coding guidelines

- Match existing package layout (`internal/k8s`, `internal/tui`, `internal/webapp`, `cmd`).
- Prefer typed Kubernetes APIs via `client-go`; avoid shelling out to `kubectl` except for intentional interactive shell flows.
- Do not commit kubeconfigs, tokens, cookie secrets, or real cluster data.
- Keep web assets embeddable (no bundler required); follow the existing React ESM style in `internal/webapp/static/`.
- Update `README.md` / `docs/` when behaviour or flags change.

## Security reports

Do **not** open public issues for vulnerabilities. Prefer a private report to the maintainer (see repository security policy / contact). Include reproduction steps, affected versions, and impact.

## Release process

Tagged releases (`vX.Y.Z`) are built by GitHub Actions (`.github/workflows/release.yml`) and publish multi-platform binaries. Maintainers cut tags after CI is green on `master`.
