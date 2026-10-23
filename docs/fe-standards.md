# Chuẩn code web (React)

Rút gọn từ `console` (margarita) của công ty. **Không** chép `packages/*`, host contract và các gate coupling
của console: chúng phục vụ tách 21 module của app lớn, quá nặng cho repo này.

## Bố cục

```
web/src/
├── main.tsx  App.tsx
├── app/                        # composition root
│   ├── router.tsx              # gom route của mọi module
│   ├── paths.ts                # một object `paths` cho mọi đường dẫn
│   ├── register-<module>.tsx   # nối module vào app (route, nav)
│   └── layout/                 # sidebar, header
├── modules/<tên>/              # mỗi màn/feature một module
│   ├── module.ts               # export: route, nav item
│   ├── paths.ts                # đường dẫn của module
│   ├── services/               # hook TanStack Query + hàm gọi API; module chỉ gọi BE qua đây
│   ├── components/             # <Name>/index.tsx
│   └── <Page>.tsx
└── shared/                     # http (axios instance), ui, lib dùng chung
```

## Luật

- TypeScript `strict`. Kiểu cho mọi response API (đặt trong `services/types.ts` của module).
- **Module chỉ gọi BE qua `services/` của chính nó**; module không import module khác (cần dùng chung → `shared/`).
- Data: TanStack Query (`useQuery`/`useMutation`), query key dạng mảng `['issues', repoId, filters]`; không fetch trong `useEffect`.
- Form: react-hook-form. Bảng: TanStack Table.
- Component: function + hooks; tên thư mục PascalCase `<Name>/index.tsx`, hook `use-<tên>.ts` (kebab-case).
- Style: Tailwind; không CSS inline trừ giá trị động. Không ép ALL-CAPS cho tiêu đề, tab, badge.
- Dashboard dày đặc kiểu ERP: header không chứa dữ liệu record; mỗi màn tối đa một nút primary.
- Có trạng thái loading (skeleton), empty và error cho mọi màn gọi API.
- File ≤200 dòng; lớn hơn thì tách component/hook.

## Test

- Vitest cho hàm thuần (mapper, format, tính cache hit %, chi phí) và hook có logic.
- Test phải fail được: đổi logic, thấy đỏ, hoàn nguyên.
