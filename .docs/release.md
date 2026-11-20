# Phát hành wj

Dành cho người bảo trì repo. Một lệnh làm hết từ kiểm tra tới bản phát hành trên GitHub:

```sh
sh .tools/release-new-wj-version/release-new-version.sh 0.2.12      # -p: cập nhật giá model (prices.json) trước; -h: xem các bước
```

Trong Claude Code: `/release 0.2.12` (skill `.claude/skills/release/`).

## Trước khi chạy

- Mọi thay đổi đã commit trên một nhánh (đang ở `main` thì script tự tạo nhánh `release-<version>`).
- `CHANGELOG.md` có mục `## <version>` ở đầu: vài dòng nói điều gì đổi với người dùng. `wj init` hiện các mục
  này cho project đang dùng kit cũ hơn.
- `gh` đã đăng nhập (`gh auth status`).

## Script làm gì

1. Kiểm tra: số version dạng `0.2.12`, cây làm việc sạch, có mục changelog, tag `v<version>` chưa có.
2. `gofmt -l .`, `go vet ./...`, `go test ./...`. Log nằm trong `.tmp/release/`.
3. Ghi số mới vào `VERSION` và `.claude/wj-kit`, commit `chore: release <version>`.
4. Đẩy nhánh, mở PR vào `main` với nội dung là mục changelog, rồi merge.
5. Cập nhật `main` ở máy, gắn tag `v<version>` và đẩy tag. Tag kích hoạt workflow `Release`
   (`.github/workflows/release.yml`): build cho Windows, macOS, Linux, các gói kit lite, và đăng bản phát hành.
6. Chờ workflow chạy xong, in link bản phát hành và số file đính kèm.

Mỗi bước in một dòng `== ...`. Lỗi thì dòng cuối là `FAIL: ...`, nói bước nào hỏng và file log nào cần xem.

## Khi hỏng

- Hỏng trước khi đẩy tag: sửa nguyên nhân rồi chạy lại cùng lệnh. Commit nâng version đã có thì không tạo lại,
  PR đang mở thì dùng lại.
- Workflow `Release` hỏng sau khi đã đẩy tag: đừng chạy lại script (tag đã có). Xem `gh run view <id> --log-failed`,
  sửa trên `main`, rồi xoá tag cũ và gắn lại: `git push origin :refs/tags/v<version>`, `git tag -d v<version>`,
  `git tag -a v<version> -m v<version>`, `git push origin v<version>`.
