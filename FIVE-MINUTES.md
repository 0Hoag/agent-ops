# Five minutes before you start

Đọc **trước hành động đầu tiên** và **ngay sau compaction**. Mỗi mục là lỗi đã xảy ra thật ở repo này.
Thêm mục mới mỗi khi agent mắc lỗi, theo khung dưới. Bộ lỗi chung: skill orchestrator-workers → `reference/pitfalls.md`.

---

## 1. Đọc kết quả sub-agent / output của worker trước

Danh sách agent trống không có nghĩa không ai làm. **Check:** đọc queue là việc đầu tiên.

## 2. Không nói con số không đo trên commit này

**Check:** chạy lại lệnh, hoặc ghi `[?]`.

## 3. Trước khi tin một check, chứng minh nó fail được

**Check:** cắm lỗi trong worktree tạm; thông báo phải nhắc đúng thứ đã cắm.

## 4. Merge ghim SHA đầy đủ; sau merge kiểm nội dung đã vào main

```bash
F=$(git diff --name-only origin/main...origin/<branch>)
git diff origin/main origin/<branch> -- $F
```

## 5. Không push vào nhánh của PR đã merge

```bash
gh pr view <n> --json state -q .state | grep -qx OPEN || exit 1
```

---

<!-- Khung cho mục mới:

## N. <luật, một dòng>

<chuyện gì xảy ra>

> Cost: <mất gì, cụ thể>

**Check:** <lệnh hoặc bước kiểm>
-->

## Luật nằm dưới tất cả

Chứng minh một thứ bên cạnh không phải là chứng minh thứ đó.

## 6. Green gates do not establish acceptance coverage

W1 SCM/store passed local gates and CI, but review found malformed GitHub paths, missing closed history,
an unconfigurable registry, incorrect status/reference mapping, and untested repository deliverables.

> Cost: two returned PRs and ten remediation issues before ingestion could begin.

**Check:** map each issue's acceptance criterion to a test that exercises its actual protocol/SQL
behavior. HTTP fixtures must assert RequestURI and query parameters, not only decoded URL.Path;
SQL tests must inspect conflict/update behavior, not only an INSERT prefix. Run golangci-lint gates
sequentially on this machine because concurrent invocations contend for its global lock.

## 7. Resolve permitted working paths from the actual brief

WS-a1 created the public client subtree repository but stopped before a PR because its clone was under
`agent-ops-wt/client-bootstrap`, even though the brief explicitly permitted that implementation path.

> Cost: one incomplete worker attempt and another dispatch before standalone client CI could be reviewed.

**Check:** distinguish canonical runtime checkout from the authorized implementation clone; verify the
existing clone's remote, clean status and HEAD, then continue in the permitted path through PR creation.

## 8. A local negative control does not prove hosted CI rejection

Client bootstrap #1 had green GitHub CI, but its closed TypeScript control #2 had no workflow runs.
The worker described it as a fork limitation although the measured head repository was the same repo.

> Cost: one returned bootstrap PR and follow-up #138 before the repository split can continue.

**Check:** inspect the control's head repository and check runs with `gh pr view`; require a hosted run
that fails at the intended step with the injected diagnostic before accepting a CI negative control.

## 9. Configuration examples must match a real consumer

Client bootstrap advertised APP_API_URL as runtime configuration while Axios used a fixed same-origin
base URL and Rspack had only a hardcoded health proxy. Independent review caught the misleading setup.

> Cost: one returned bootstrap PR and focused documentation follow-up #139.

**Check:** trace each documented environment variable to its loader and consumer on the exact PR head;
describe current behavior when environment-aware configuration belongs to a later wave.

## 10. Issue transfer does not preserve labels automatically

WS-a2 transferred root #132 to client #4, but the destination retained only area:client and lost its
status, priority, type and wave labels. The attempt ended before producing a complete mapping.

> Cost: a fresh split-completion retry and destination metadata repair before dispatch can continue.

**Check:** provision destination labels first; compare source and destination metadata immediately after
each transfer, record old/new URLs, and verify body newlines and cross-repository dependency links.

## 11. A repeat-write test name is not an idempotency assertion

W1cr4's sync test repeated INSERT-prefix SQL mocks but asserted neither conflict keys nor normalized
values and omitted pipelines, repeating the acceptance gap documented in item 6.

> Cost: another returned ingestion PR and focused follow-up #140 despite green API gates and CI.

**Check:** inspect conflict targets/update clauses and bound normalized values for every entity on both
passes; a changed identity or state must fail the relevant assertion in a disposable negative control.

## 12. Issue migration must preserve the contract, not only the title

WS-a2 retry replaced fourteen transferred client issue bodies with generic boilerplate, removing
deliverable-specific acceptance and dependencies; fourteen also lacked wave labels.

> Cost: split PR returned and #142 required to reconstruct contracts from GitHub edit histories.

**Check:** compare every destination body against its pre-migration `userContentEdits` history,
preserving measurable acceptance and fully qualified dependency links; audit all label dimensions
and real newlines before claiming transfer completion.
