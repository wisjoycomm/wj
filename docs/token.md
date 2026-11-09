# Token

Đọc thẳng file phiên của Claude Code và Codex trên máy này. Không cần proxy, không gọi
mạng, tính cả phiên mở từ Orca. Các worktree của một repo gộp về một project.

## Lệnh

```text
wj top                    bảng trực tiếp, project vừa chạy lên đầu (Ctrl+C để thoát)
wj top -once              in một lần
wj overlay                cửa sổ nhỏ luôn nổi trên màn hình
wj overlay stop           đóng overlay
wj usage                  bảng chi tiết theo project + app
wj usage -by project-model   từng model trong mỗi project
wj usage -by app          theo app + model; còn có -by project, -by model
wj usage -project game -app codex
```

`-since 168h` đổi khoảng thời gian (mặc định 24 giờ). `wj usage -all` lấy toàn bộ.

Trong menu `wj`: AI Agent → Xem token, có đủ `wj top`, mở/đóng overlay và `wj usage`.

## wj top

Cùng báo cáo với overlay mở rộng, thêm số lượt và cache tách đọc/ghi:

```text
  PROJECT            APP     LẦN CUỐI  MODEL              LƯỢT   VÀO      RA  ĐỌC CACHE  GHI CACHE   TỔNG    ≈USD
● endlesswinter      claude  vừa xong  2 model             444   906  193.5k      49.3M     609.0k  50.1M   $9.11
  └                  claude  vừa xong  claude-sonnet-5-5   413   830  175.5k      47.2M     550.3k  47.9M   $7.85
  └                  claude  2 phút    claude-opus-5-5      31    76   18.0k       2.1M      58.7k   2.2M   $1.26
○ game               claude  đang mở                         0     0       0          0          0      0      $0
```

- `●` dùng token trong 2 phút, `○` có phiên Claude Code đang mở (kể cả chưa dùng token).
- Project dùng nhiều model có thêm một dòng `└` cho mỗi model.
- Terminal hẹp thì gộp cache thành một cột, hẹp nữa thì bỏ cột MODEL (model ghi sau `└`).
  Terminal thấp thì bớt dòng, ghi `...và N project khác`.
- Dùng nhiều app (Claude, Codex) thì có thêm dòng `Theo app`.

## Tổng cả máy

`wj top` có dòng tổng mọi project:

```text
Cả máy: 24 giờ 7.7M · 7 ngày 173.6M · 30 ngày 812.1M · toàn bộ 986.3M
```

Overlay hiện tổng cả máy trong khoảng đang xem, thêm 7 ngày và 30 ngày.

## Overlay

Chỉ cần mở một lần, không phải setup hay thêm tay từng project. Project mới và session
đang chạy dở tự hiện sau tối đa 5 giây, không cần khởi động lại session.

Hiện tối đa 20 project:

- **Claude Code**: session đang mở, kể cả khi chưa dùng token.
- **Codex**: có dùng token trong 10 phút gần nhất. Codex không ghi lại session đang mở, nên
  session Codex nghỉ lâu hơn sẽ tạm ẩn cho đến lượt dùng tiếp theo.

Chấm xanh là project vừa dùng token trong 2 phút. Số token của mỗi dòng tính trong khoảng
đang xem (mặc định 24 giờ). Dòng đầu là tổng cả máy, ví dụ `24h 7.7M · 7d 173.6M · 30d 812.1M`.

Nền overlay trong suốt để thấy phần bên dưới, chữ vẫn rõ nét.

Bấm đúp vào overlay (hoặc chuột phải, chọn Mở rộng) để xem báo cáo: app, lần dùng gần nhất,
model, token vào, ra, cache và số tiền quy đổi của từng project, thêm cả project đã đóng trong
khoảng đang xem và tổng cả máy. Project dùng nhiều model có thêm một dòng cho mỗi model; nếu có
quá nhiều dòng thì project đó chỉ ghi `3 model`, xem đủ bằng `wj usage -by project-model`.
Bấm đúp lần nữa để thu gọn. Mỗi lần mở overlay đều bắt đầu ở dạng thu gọn.

## Cảnh báo

```text
wj overlay -warn 5M
```

Dòng tổng hiện `16.5M/5M`, chuyển vàng từ 80% ngưỡng và đỏ khi vượt. Ngưỡng do mình tự đặt, tính
trên tổng cả máy trong khoảng đang xem (`-since`). Mặc định không cảnh báo.

Đây không phải limit thật của nhà cung cấp. Số token không cho biết còn lại bao nhiêu: gói Claude
xem bằng `/usage`, còn gateway xem trên trang của gateway.

Kéo để đổi chỗ, chuột phải để đóng. Vị trí được lưu cho lần mở sau. Hiện chỉ có trên
Windows; macOS và Linux dùng `wj top` trong một cửa sổ terminal nhỏ.

## Đọc số thế nào

- **VÀO**: token prompt mới.
- **RA**: token trả lời.
- **ĐỌC CACHE**: thường rẻ hơn nhiều so với VÀO, và thường chiếm phần lớn TỔNG.
- **TỔNG**: cộng cả bốn loại, nên không phải số tiền. Giá thật do gateway quyết định.
- **QUY ĐỔI** (`≈USD` trên overlay): số tiền nếu trả theo giá API niêm yết.

## Quy đổi tiền

Có trong `wj usage`, `wj top` và overlay mở rộng, không có ở overlay thu gọn.

Đây là phép quy đổi, không phải số tiền thật phải trả: gói Pro/Max là phí cố định, còn gateway
có giá riêng. Dùng để so sánh project và model với nhau.

- Mỗi lượt gọi tính theo đúng model, loại token, cache 5 phút hay 1 giờ, và mức giá theo độ dài prompt.
- Bảng giá lấy từ danh sách công khai của OpenRouter, nhúng sẵn trong `wj` nên không gọi mạng.
  Ngày của bảng giá in dưới bảng. Cập nhật cùng bản phát hành bằng `scripts/update-prices.sh`.
- Model không có trong bảng giá không bị đoán: `?` là chưa có giá, `$12.00+` là còn token chưa
  tính. Model có tên riêng của gateway thường rơi vào trường hợp này.
- Không tính fast mode, batch, inference theo vùng và phí công cụ như web search.

## So với /usage của Claude Code

| | `/usage` | `wj` |
|---|---|---|
| Cho biết | Còn bao nhiêu % hạn mức gói | Đã dùng bao nhiêu token |
| Theo project | Không | Có |
| Codex | Không | Có |
| Biết sắp hết hạn mức | Có | Không |
| Tính cả máy khác | Có, theo tài khoản | Không, chỉ máy này |

## Giới hạn

- Chỉ tính được khi file phiên còn trên máy.
- Số liệu của từng máy riêng, không cộng giữa các máy.
- App khác (OpenCode...) đo bằng `wj proxy run` và `wj proxy usage`. Xem `wj help proxy`.
