# Roles

<!-- Bản rút gọn cho repo. Bản đầy đủ: skill $orchestrator-workers → references/roles.md. -->

| Vai | Làm | Không làm |
| --- | --- | --- |
| Owner (người) | quyết định, credentials, bước không đảo ngược | |
| PM | đo, viết issue, sắp thứ tự, giữ status board (issue ghim) | code, merge |
| Coordinator | lấy issue, brief, chạy worker, kiểm tra, merge ghim SHA | tự đặt ưu tiên, làm thay worker |
| Implementer | 1 issue / 1 nhánh / 1 PR, báo phần chưa làm | merge, đụng issue khác |
| Reviewer | chỉ đọc, tìm false pass | sửa, commit, merge |

## Năm phút đầu

```bash
ls .agent-runs/ 2>/dev/null          # worker nào đã báo về? (<wave>.md, <wave>.jsonl)
git fetch origin main && git log origin/main --oneline -5
gh run list --branch main --limit 1
gh pr list && gh issue list --label "status:in progress"
<lệnh gate>
```

## Vòng lặp coordinator

1. Đọc lại issue ngay bây giờ.
2. Không PR nào khác đụng cùng file (`gh pr diff <n> --name-only`).
3. `node scripts/issue-status.mjs <n> "in progress"` + comment `Status: dispatched ...`.
4. Viết `briefs/<wave>.md` (template trong skill) → `scripts/run-worker.sh <wave> <branch> briefs/<wave>.md`. Không làm song song cùng việc.
5. Đọc `.agent-runs/<wave>.md` → chạy lại 1-2 con số; "tests pass" phải là lệnh gate của repo trên head của PR.
6. Reviewer độc lập nếu đụng CI/gate/auth.
7. `gh pr checks <pr>` xanh → `gh pr merge <pr> --squash --match-head-commit <sha> --delete-branch`.
8. Kiểm nội dung đã vào main; `git worktree remove`.
9. Issue đóng qua `Closes #n`; cập nhật status board.

## Gửi lại khi

control không fail được · số không có phương pháp · claim UI không có lần chạy thật · tiêu chí bị bỏ
không báo · phạm vi phình · thiếu `Closes #n`.

## Hỏi owner khi

quyết định sản phẩm · khó đảo ngược / rời máy · credentials. Còn lại: tự quyết, làm, báo.
