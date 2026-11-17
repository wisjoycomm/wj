# Cài máy mới

Gõ `wj`, chọn **Ứng dụng & môi trường** rồi **Cài máy mới** để tool hỏi từng bước. Hoặc chạy từng lệnh:

```text
wj apps list                     xem app nào đã cài
wj apps install claude,codex     cài app còn thiếu
wj apps update codex,orca        cập nhật app
wj tools install cli             cài bộ công cụ dòng lệnh (hỏi chọn nhóm)
wj tools install wm              cài quản lý cửa sổ (không bắt buộc)
wj tools install wsl             cài WSL2 + Ubuntu (Windows)
wj update                        cập nhật wj
```

Thêm `-dry-run` để xem trước, không thay đổi gì.

Cài xong mà gõ lệnh không thấy thì đóng terminal và mở cái mới.

## Đổi tên từ setup-gateway

Máy đã cài bản cũ vẫn chạy được. Lần chạy đầu, tool chuyển `~/.setup-gateway` sang
`~/.wj` và tạo thêm lệnh `wj` cạnh lệnh cũ.
