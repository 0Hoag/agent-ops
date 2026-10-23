# Status

Commit của các con số: chưa có (repo mới). Đánh dấu: `[M]` đo được (kèm lệnh + commit) · `[?]` chưa kiểm · `[X]` rút lại.

## Trạng thái

| Đợt | Trạng thái | PR | Ghi chú |
| --- | --- | --- | --- |
| W0a api | todo | | |
| W0b web | todo | | |
| W0c ci | todo | | |

## Chi phí theo đợt

Lấy từ tổng token mà `scripts/run-worker*.sh` in ra khi xong (Codex: `turn.completed.usage`; OpenCode: `step_finish.part.tokens`).

| Đợt | Harness | Worker | Model | Input | Cached input | Cache write | Output | Cache hit % | $ | Issue | Gửi lại |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| | | | | | | | | | | | |

Cache hit % = cached input / (input + cached input) nếu `input_tokens` không gồm phần cached; kiểm định nghĩa
trường một lần rồi ghi lại ở đây `[?]`.

## Lỗi agent mới (chuyển vào FIVE-MINUTES.md khi lặp lại)

-
