# Development

## Releases

lsm uses a single tag-driven release pipeline,
`.github/workflows/release.yml`, with three trigger paths:

- **Auto path** — every push to `main`. The workflow inspects the
  Conventional Commit subjects since the last tag, decides the next
  version, pushes a new `vX.Y.Z` annotated tag, then builds the
  cross-platform binaries and publishes the GitHub Release.
- **Manual major path** — a maintainer pushes a `vX.0.0` tag from
  their workstation (`git tag -a vX.0.0 -m ...; git push origin vX.0.0`).
  The version-computation job is skipped and the build/publish job
  runs against the pushed tag.
- **Back-fill path** — `workflow_dispatch` from the Actions UI with
  a `tag` input. For back-filling a missed release against an
  existing tag.

The runtime version is sourced from the tag at build time
(`-X github.com/llbbl/lsm/internal/cmd.Version=$TAG`). There is no
tracked `VERSION` file.

### Automatic releases

Every push to `main` is evaluated by the workflow's `version` job.
It runs `go vet` and `go test -count=1 ./...` as a sanity check, then
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
the auto path — they produce a minor bump. Major releases are gated
behind an explicit maintainer action.

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
4. The `push: tags` trigger on `release.yml` fires (authenticated by
   the maintainer's own credentials, not `GITHUB_TOKEN`), the version
   job is skipped, and the release job builds and publishes the
   artifacts.

Do not edit the workflow to "just allow majors this once" — push the
tag by hand instead.

### Back-filling a missed release

If the build/publish step fails after a tag was created (e.g. a
GitHub outage, a transient build issue that's since been fixed),
re-run the release against the existing tag:

1. Go to Actions → Release → "Run workflow".
2. Supply the existing tag name (e.g. `v0.1.4`) as the `tag` input.
3. The `workflow_dispatch` trigger fires, the version job is skipped,
   and the release job builds against the supplied tag.

### Why a single workflow file

Earlier iterations split this into two files (`auto-release.yml` for
the auto path, `release.yml` for tag-push and dispatch). That split
existed because GitHub Actions suppresses downstream workflow
triggers for pushes authenticated with `GITHUB_TOKEN` (anti-recursion):
when `auto-release.yml` pushed a tag, the separate `release.yml`
`push: tags` trigger did not fire, so the build/publish steps had to
be duplicated in both files.

Consolidating into one file with three trigger paths and a shared
build/publish `release` job eliminates the duplication. The auto
path's `version` job pushes the tag and feeds it directly to the
release job via `needs.version.outputs.tag`; the tag-push and
dispatch paths skip the version job entirely and feed the tag in via
a `resolve-tag` step. The `always()` guard on the release job's `if:`
condition is what lets it run even when the version job is skipped.
