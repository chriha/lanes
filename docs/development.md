# Development

```sh
make build        # ./bin/lanes
make test         # go vet + go test
make release      # tag and push the next release
```

## Releasing

`make release` tags the next release from the [Conventional Commits](https://www.conventionalcommits.org) since
the last `v*` tag: a `feat` bumps the minor version, anything else the patch; the first release is `v1.0.0`. It
checks that you are on `main` with a clean tree, in sync with `origin/main`, that the tag doesn't exist yet and that
the `ci` workflow succeeded on HEAD (needs `gh`). It then shows the version and the commits and asks before it
creates and pushes an annotated tag. The pushed tag runs the release workflow, which tests, builds the binaries with
GoReleaser and publishes the GitHub release with a grouped changelog.

| Variable     | Effect                                                                                    |
|--------------|-------------------------------------------------------------------------------------------|
| `DRY_RUN=1`  | show the version and commits, create no tag                                               |
| `MAJOR=1`    | allow a major bump (a `type!:` commit or a `BREAKING CHANGE:` footer); refused otherwise, as v2+ needs a `/v2` module path for `go install …@latest` to see it |
| `V=vX.Y.Z`   | tag this version instead; must be greater than the last release                           |
| `SKIP_CI=1`  | skip the CI and `HOMEBREW_TAP_TOKEN` checks                                               |

### One-time Homebrew setup

The release workflow also pushes the formula to the tap `chriha/homebrew-lanes` (install: `brew install
chriha/lanes/lanes`). That needs a fine-grained personal access token with *Contents: read and write* on the tap
repo only, stored as the `HOMEBREW_TAP_TOKEN` secret of this repo: `gh secret set HOMEBREW_TAP_TOKEN`. `make
release` refuses to tag while the secret is missing.

Example: `make release DRY_RUN=1`.

The code is organised by concern under `internal/`: `config` (TOML, project overlay, in-app overrides),
`provider` (GitLab, GitHub, command), `filter` (expressions), `session` (tmux, Claude hooks), `history`, `cache` and
`ui` (Bubble Tea). Design notes: [docs/design.md](design.md).

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss), [Bubbles](https://github.com/charmbracelet/bubbles),
[expr](https://github.com/expr-lang/expr) and [BurntSushi/toml](https://github.com/BurntSushi/toml).
