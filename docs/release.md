# Local semantic release tooling

`scripts/release.sh --dry-run` calculates a deterministic version from the Git
history and prints the proposed version followed by its changelog. It is
read-only: it does not move refs, stage files, modify the worktree, or contact
an external service. `--dry-run` is the default. Use `scripts/release.sh
--write` to write the resulting text to `CHANGELOG.md` locally.

The latest tag matching `vMAJOR.MINOR.PATCH` is the base. Without such a tag,
the base is `0.1.0`. `feat` makes a minor release, `fix`, `perf`, `refactor`,
and `revert` make a patch release, and `BREAKING CHANGE:` or a `!` before the
colon makes a major release. Major wins over minor, and minor wins over patch.
Other commit types do not create a release. Merge commits and merge
bookkeeping are omitted from the changelog. The tool reads local Git history
only; publishing tags, uploading artifacts, and CI are intentionally outside
its scope.

Run the disposable verification with:

```bash
bash scripts/test-release.sh
bash -n scripts/release.sh scripts/test-release.sh
git diff --check
```
