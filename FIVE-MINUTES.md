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
