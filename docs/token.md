# Token

Đọc thẳng file phiên của Claude Code, Codex và OpenCode trên máy này (Claude, Codex: file `.jsonl`;
OpenCode: file cơ sở dữ liệu `opencode.db`). Không cần proxy, không gọi mạng, không dùng token, tính cả
phiên mở từ Orca. Các worktree của một repo gộp về một project.

## Lệnh

```text
wj top                    bảng trực tiếp, project vừa chạy lên đầu (Ctrl+C để thoát)
wj top -once              in một lần
wj overlay                cửa sổ nhỏ luôn nổi trên màn hình
wj overlay stop           đóng overlay
wj usage                  báo cáo hôm nay, dòng gõ lệnh ở dưới để đổi kỳ/bộ lọc (xem "Xem theo ngày")
wj usage week             in một lần rồi thoát (cho script); còn lastweek, month, 2026-11-09...
wj usage -since 24h       bảng cũ theo project + app
wj usage -by project-model   từng model trong mỗi project
wj usage -by app          theo app + model; còn có -by project, -by model
wj usage -project game -app codex
```

`-since 168h` đổi khoảng thời gian của bảng cũ (mặc định 24 giờ). `wj usage -all` lấy toàn bộ.

Trong menu `wj`: AI Agent → Xem token, có đủ `wj top`, mở/đóng overlay và `wj usage`.

## Xem theo ngày, tuần, tháng

Chỉ cần gõ `wj usage`. Phía trên là báo cáo, phía dưới là danh sách lệnh (dropdown) và dòng nhập `>`, giống
menu `/` của Claude Code. Chọn lệnh nào thì báo cáo đổi theo lệnh đó ngay, không cần gõ lại cả lệnh.

```text
... báo cáo của kỳ đang xem ...
Lệnh   ↑↓ chọn và xem ngay · Tab điền · Enter chốt · Esc huỷ
  ● today      hôm nay  2026-11-10 T3                              4 session · 16.5M · $13.45
    yesterday  hôm qua  2026-11-09 T2                              19 session · 737.6M · $398
    week       tuần này  2026-11-09 → 2026-11-15                   19 session · 754.1M · $411
    lastweek   tuần trước  2026-11-02 → 2026-11-08                 77 session · 7.7B · $3,211
    month      tháng này  2026-11-01 → 2026-11-30                  95 session · 8.4B · $3,644
    lastmonth  tháng trước  2026-10-01 → 2026-10-31                76 session · 5.2B · $5,707
    agents     liệt kê từng worker (subagent) dưới session của nó
    top        số session hiện ra (đang: tự vừa)
▼ còn 5 dòng nữa, bấm ↓ để xem · gõ thêm để xem kỳ khác: 2026-11-05 · 2026-11 · 14d
> 
```

| Phím | Tác dụng |
|---|---|
| `↑` `↓` | chọn lệnh trong danh sách. Báo cáo bên trên đổi sang kết quả của lệnh đó ngay (xem trước) |
| `Enter` | chốt lệnh đang chọn. Lệnh có giá trị (`top`, `project`, `app`) thì mở danh sách giá trị của nó |
| `Tab` | điền lệnh đang chọn vào dòng nhập |
| gõ chữ | lọc danh sách, dòng đầu tự được chọn: gõ `la` còn `lastweek`, `lastmonth` |
| `Esc` | bỏ xem trước; bấm tiếp thì xoá dòng nhập; bấm tiếp nữa thì thoát |
| `PgUp` `PgDn` `Home` `End` | cuộn báo cáo khi dài hơn màn hình |
| `Ctrl+U`, `Ctrl+C`, `q` | xoá dòng nhập, thoát, thoát |

Dấu `●` là giá trị đang dùng, `›` là dòng đang chọn. Danh sách của `project` liệt kê đúng các project có dữ liệu trong
kỳ đang xem, kèm dung lượng và ≈USD của từng project. Thoát xong, báo cáo cuối cùng được in ra màn hình thường
để bạn kéo lên xem hoặc copy.

Màn hình này cần terminal đủ lớn (từ 70 cột và 16 dòng). Nhỏ hơn hoặc terminal không hỗ trợ thì `wj usage` quay về
kiểu gõ lệnh từng dòng như trước. Lần mở đầu tiên đọc cả tháng nên mất vài giây; mỗi lần bấm phím sau đó khoảng 0.1 giây.

Đầu báo cáo có bảng **Các kỳ**: sáu kỳ cạnh nhau (hôm nay, hôm qua, tuần này, tuần trước, tháng này, tháng trước)
với số session, lượt, token và ≈USD, kỳ đang xem đánh dấu `▶`. Bảng đi theo bộ lọc `project`/`app` đang chọn. Kỳ gõ
tay (`2026-11-05`, `7d`...) tự thêm một dòng riêng.

```text
Các kỳ (gõ số hoặc tên kỳ để chuyển; ▶ là kỳ đang xem):
   KỲ           TÊN          NGÀY                     SESSION   LƯỢT    TỔNG  QUY ĐỔI
▶  1 today      hôm nay      2026-11-09 T2                 19   4982  731.8M     $396
   2 yesterday  hôm qua      2026-11-08 CN                  -      -       -        -
   3 week       tuần này     2026-11-09 → 2026-11-15       19   4982  731.8M     $396
   4 lastweek   tuần trước   2026-11-02 → 2026-11-08       77  40658    7.7B   $3,211
   5 month      tháng này    2026-11-01 → 2026-11-30       95  45809    8.4B   $3,628
   6 lastmonth  tháng trước  2026-10-01 → 2026-10-31       76  31514    5.2B   $5,707
```

Bảng này chỉ có ở màn hình `wj usage` (không kỳ, không cờ). `wj usage week` và các lệnh có kỳ in một lần như cũ, không
có bảng.

| Gõ ở dòng `>` | Tác dụng |
|---|---|
| `1`–`6` | nhảy tới kỳ có số đó trong bảng "Các kỳ" |
| `today`, `yesterday` | hôm nay, hôm qua |
| `week`, `lastweek` | tuần này, tuần trước; tuần bắt đầu thứ Hai |
| `month`, `lastmonth` | tháng này, tháng trước |
| `7d`, `30d` | N ngày gần nhất, tính cả hôm nay |
| `2026-11-09`, `2026-11` | một ngày, một tháng |
| `2026-11-01..2026-11-09` | khoảng ngày bất kỳ (gồm cả hai đầu) |
| `agents` / `agents off` | bật/tắt liệt kê từng worker dưới session của nó |
| `top 30`, `top 0`, `top auto` | số session hiện ra (0 = tất cả, auto = vừa màn hình) |
| `project TÊN`, `project all` | lọc theo project |
| `app claude`, `app codex`, `app all` | lọc theo app |
| `reset`, `help`, `q` | về mặc định, xem lệnh, thoát |

Bảng dưới là các lệnh dùng được cả khi gõ tay ở dòng nhập (kể cả kiểu gõ từng dòng khi terminal nhỏ):


Báo cáo chia theo ngày dương lịch của máy (không phải 24 giờ gần nhất). Nhiều ngày thì có bảng "Theo ngày"; một
ngày thì chỉ liệt kê session. Số session hiện ra tự vừa màn hình, session nhỏ nhất bị ẩn và ghi "còn N session nhỏ
hơn" (`top 0` để xem hết).

Không có terminal (chạy qua pipe, trong script) hoặc có kỳ/cờ đi kèm thì `wj` in một lần rồi thoát, như cũ:

```text
wj usage lastweek -top 30 -agents -project game -app claude
```

Kỳ có thể đứng trước hoặc sau các cờ.

Với `agents` (hoặc `-agents`), mỗi worker (subagent) là một dòng `└` ngay dưới session đã chạy nó, kèm mô tả việc
Claude Code ghi lúc giao, model, số lượt, token, context lớn nhất và ≈USD. Mỗi session hiện tối đa 15 worker
đắt nhất.


```text
SESSION       PROJECT            LƯỢT    VÀO      RA  ĐỌC CACHE  GHI CACHE    TỔNG    %    AGENT PHỤ  CTX MAX  QUY ĐỔI  MODEL          TIÊU ĐỀ
e05f2e8a      endlesswinter       102   59.9k  233.5k      11.3M     732.8k   12.3M  35%    11.1M (2)   218.6k   $10.59  opus-5-5 +1    Task còn lại
  └ af232f0d  general-purpose      60   31.0k  120.2k       5.0M     310.1k    5.5M  16%                191.1k    $4.96  opus-5-5       T4.6 scout/attack/plunder
```

Cuối báo cáo luôn có cây tổng hợp: tổng, rồi từng model, rồi từng project dùng model đó. Cây theo đúng kỳ và bộ lọc
đang chọn; `cc/claude-opus-5-5` và `claude-opus-5-5` gộp làm một model.

```text
Tổng hợp theo model, rồi theo project:
MODEL / PROJECT          LƯỢT     VÀO      RA  ĐỌC CACHE  GHI CACHE    TỔNG     %  QUY ĐỔI
TỔNG                     4894  812.5k    7.2M     669.5M      36.0M  713.4M  100%     $387
├─ opus-5-5              2883  480.1k    5.4M     406.5M      21.4M  433.9M   61%     $305
│  ├─ endlesswinter      2259  466.2k    5.0M     289.4M      19.0M  313.8M   44%     $258
│  └─ endless-winter-be   400     810  212.2k      79.4M     795.4k   80.4M   11%   $24.31
└─ sonnet-5-5            1974  332.3k    1.8M     262.1M      14.2M  278.3M   39%   $81.60
   └─ endlesswinter      1510  295.6k    1.2M     181.3M       9.7M  192.5M   27%   $55.70
```

Mỗi dòng session cho biết:

- **SESSION:** 8 ký tự đầu của id phiên. Đối chiếu với tên file `~/.claude/projects/<project>/<id>.jsonl`.
- **VÀO, RA, ĐỌC CACHE, GHI CACHE, TỔNG:** đủ từng loại token của session, giống bảng cây ở cuối báo cáo. TỔNG là cộng cả bốn.
- **%:** phần của session trong cả kỳ.
- **AGENT PHỤ:** token của các subagent (worker) mà session đã chạy. Đã nằm trong TỔNG, nên session nào có
  AGENT PHỤ gần bằng TỔNG nghĩa là phần lớn tiền tiêu ở worker, không ở chat chính.
- **CTX MAX:** prompt lớn nhất của một lượt gọi. Mỗi lượt đọc lại cả prompt đó, nên context lớn nhân với nhiều
  lượt là chỗ tốn. Xem [context.md](context.md).
- **MODEL:** model dùng nhiều nhất, `+N` là số model khác.
- **TIÊU ĐỀ:** tên chat do Claude Code đặt (Codex không có thì để `-`).

**Cửa sổ hẹp:** bảng session tự thu cho vừa chiều rộng. Cột TIÊU ĐỀ bị cắt ngắn trước (còn tối thiểu 12 ký tự), rồi ẩndần MODEL, CTX MAX, AGENT PHỤ, %, LƯỢT, PROJECT; cột token (VÀO, RA, ĐỌC CACHE, GHI CACHE, TỔNG) và QUY ĐỔI luôn được giữ.Báo cáo ghi một dòng "cửa sổ hẹp: đã ẩn cột ...". Kéo rộng cửa sổ (khoảng 150 cột) để xem đủ mọi cột.

Lưu ý:
- **Subagent tính vào session đã chạy chúng,** không thành session riêng trong danh sách chính. Muốn xem riêng
  từng worker thì gõ `agents`. Con số `(26)` ở cột AGENT PHỤ là số worker của session đó.
- **Ngày tính theo giờ máy:** một lượt gọi lúc 03:00 sáng giờ Việt Nam thuộc ngày hôm đó, dù theo UTC là hôm trước.
- **Phiên cũ có thể thiếu:** Claude Code xoá file phiên cũ theo `cleanupPeriodDays`. Nếu kỳ bạn chọn bắt đầu trước
  file phiên cũ nhất trên máy, báo cáo ghi một dòng lưu ý.
- **Một session chạy qua nhiều ngày** hiện ở mỗi ngày với phần của ngày đó trong bảng "Theo ngày", nhưng trong bảng
  session thì tính cả kỳ.

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
- Dùng nhiều app (Claude, Codex, OpenCode) thì có thêm dòng `Theo app`.

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

Token tăng vọt bất thường: xem [Giảm context](context.md).

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
- **Cursor và Antigravity chưa đo được:** Cursor ghi token trên máy chủ của họ chứ không trong file trên máy;  Antigravity có số trong cơ sở dữ liệu nhưng định dạng chưa có tài liệu, chưa đối chiếu được. Hai app này  không có mặt trong `wj usage` và `wj top`. Báo cáo chỉ gồm các app ghi trong dòng đầu.- App khác đo bằng `wj proxy run` và `wj proxy usage`. Xem `wj help proxy`.

## OpenCode

OpenCode lưu mọi phiên trong một file SQLite, `~/.local/share/opencode/opencode.db` (đổi bằng `OPENCODE_DATA_DIR`
hoặc `XDG_DATA_HOME`). `wj` mở file ở chế độ chỉ đọc, đọc cả file `-wal` kèm theo nên thấy cả những lượt mới nhất,
kể cả khi OpenCode đang chạy.

Chỉ đọc các tin nhắn trả lời của mô hình có số token (VÀO, RA, suy luận, đọc cache, ghi cache), tên model và thư mục
làm việc của session; không đọc nội dung tin nhắn. Hai quy ước:

- **Suy luận (`reasoning`) được cộng vào RA,** vì Claude và Codex gộp chung. Số RA của OpenCode vì vậy lớn hơn số
  `output` mà `opencode stats` in ra.
- **Session con tính vào session cha,** như worker của Claude: xem bằng `agents`.

Lượt lỗi hoặc bị ngắt (không có token) không được tính. Đối chiếu: `opencode stats` ghi "313 steps · 9m tokens", `wj`
tính 288 lượt, 9,0M token trên cùng dữ liệu; 25 lượt chênh là lượt không có token.
