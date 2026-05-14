# Development

## Releases

lsm uses a tag-driven release pipeline. Two workflows participate:

- `.github/workflows/auto-release.yml` — runs on pushes to `main`,
  decides the next version from Conventional Commit messages since
  the last tag, and pushes a new `vX.Y.Z` annotated tag.
- `.github/workflows/release.yml` — triggers on `v*` tag pushes,
  builds cross-platform binaries with the tag baked in via ldflags,
  generates a changelog with `git-cliff`, and publishes the GitHub
  Release.

The runtime version is sourced from the tag at build time
(`-X github.com/llbbl/lsm/internal/cmd.Version=$TAG`). There is no
tracked `VERSION` file.

### Automatic releases

Every push to `main` is evaluated by `auto-release.yml`. The workflow
runs `go vet` and `go test -count=1 ./...` as a sanity check, then
inspects the Conventional Commit subjects since the last tag:

| Commit prefix                                      | Bump  |
|----------------------------------------------------|-------|
| `feat:`, `feat(scope):`, `feat!:`, `feat(scope)!:` | minor |
| `BREAKING CHANGE:` footer (under a `feat` commit)  | minor |
| `fix:`, `perf:`, `refactor:`, `chore:`             | patch |
| anything else (`docs`, `style`, `test`, etc.)      | skip  |

If there are no new commits, or no release-worthy commits, the
workflow exits cleanly without tagging.

### Major releases are MANUAL

`feat!:` and `BREAKING CHANGE:` do **not** trigger a major bump under
the automated workflow — they produce a minor bump. Major releases
are gated behind an explicit maintainer action.

**Rationale**: while the project is iterating quickly, a stray `feat!:`
commit would otherwise produce a noisy and misleading 2.0/3.0/4.0
release. Manual gating keeps the major version intentional.

**Procedure** (run by hand on a clean checkout of `main`):

1. Decide the new major version (e.g. `v2.0.0`).
2. Tag from `main`:
   ```bash
   git tag -a v2.0.0 -m "release: v2.0.0"
   ```
3. Push the tag:
   ```bash
   git push origin v2.0.0
   ```
4. `release.yml` picks up the tag push and builds the release
   artifacts. Alternatively, draft a GitHub Release manually via the
   UI against the tag; any release-on-publish workflows will fire.

Do not edit the auto-release workflow to "just allow majors this
once" — push the tag by hand instead.
