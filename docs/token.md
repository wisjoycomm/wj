# Token

Đọc thẳng file phiên của Claude Code, Codex, OpenCode và Antigravity trên máy này (Claude, Codex: file `.jsonl`;
OpenCode: file cơ sở dữ liệu `opencode.db`; Antigravity: một file SQLite cho mỗi cuộc trò chuyện). Không cần proxy, không gọi mạng, không dùng token, tính cả
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
wj usage blocks           khối 5 giờ, tốc độ đốt và dự kiến hết khối (xem "Khối 5 giờ")
wj usage blocks -limit 5M giới hạn token một khối, cảnh báo từ 80%
wj usage week -json       JSON thay vì bảng (xem "JSON")
wj usage analytics week   token đi đâu, vì sao tốn (xem "Phân tích"); không ghi kỳ = hôm nay
```

`-since 168h` đổi khoảng thời gian của bảng cũ (mặc định 24 giờ). `wj usage -all` lấy toàn bộ.

Trong menu `wj`: AI Agent → Xem token, có đủ `wj top`, mở/đóng overlay và `wj usage`.

## Xem theo ngày, tuần, tháng

Chỉ cần gõ `wj usage`. Phía trên là báo cáo, phía dưới là danh sách lệnh (dropdown) và dòng nhập `>`, giống
menu `/` của Claude Code. Chỉ có ba kiểu kỳ: **ngày, tuần, tháng**. `←` `→` lùi/tiến một kỳ cùng kiểu (hôm qua, tuần
trước, tháng trước...). Danh sách lệnh chỉ nhận phím khi bấm `Tab` để vào nó; ngoài danh sách, `↑` `↓` (và con lăn
chuột) chỉ cuộn báo cáo, không đổi kỳ.

```text
... báo cáo của kỳ đang xem ...
Lệnh   ← → kỳ trước/sau · Tab vào danh sách · ↑↓ cuộn · Esc thoát
  ● day      theo ngày: hôm nay  2026-11-10 T3                    7 session · 536.7M · $319
    week     theo tuần: tuần này  2026-11-09 → 2026-11-15         9 session · 1.2B · $681
    month    theo tháng: tháng này  2026-11-01 → 2026-11-30      66 session · 8.7B · $3,826
    agents   liệt kê từng worker (subagent) dưới session của nó
    open     chỉ các phiên đang mở lúc này (Claude Code), bận hay rảnh
    top      số session hiện ra (đang: tự vừa)
▼ còn 5 dòng nữa, bấm ↓ để xem · gõ thêm để xem kỳ khác: 2026-11-05 · 2026-W45 · 2026-11 · 14d
> 
```

| Phím | Tác dụng |
|---|---|
| `←` `→` | kỳ trước / kỳ sau, cùng kiểu với kỳ đang xem (ngày, tuần, tháng). Không đi quá hôm nay |
| `Tab` | vào danh sách lệnh, đứng ở dòng đang dùng; đang ở danh sách thì điền lệnh vào dòng nhập |
| `↑` `↓` | ngoài danh sách: cuộn báo cáo. Trong danh sách: chọn lệnh, báo cáo đổi theo ngay (xem trước) |
| `Enter` | chốt lệnh đang chọn. Lệnh có giá trị (`top`, `project`, `app`) thì mở danh sách giá trị của nó |
| gõ chữ | vào danh sách và lọc nó, dòng đầu tự được chọn: gõ `la` còn `lastweek`, `lastmonth` |
| `Esc` | rời danh sách; bấm tiếp thì xoá dòng nhập; bấm tiếp nữa thì thoát |
| `PgUp` `PgDn` `Home` `End` | cuộn báo cáo theo trang khi dài hơn màn hình |
| `Ctrl+U`, `Ctrl+C`, `q` | xoá dòng nhập, thoát, thoát |

Dấu `●` là giá trị đang dùng, `›` là dòng đang chọn. Danh sách của `project` liệt kê đúng các project có dữ liệu trong
kỳ đang xem, kèm dung lượng và ≈USD của từng project. Thoát xong, báo cáo cuối cùng được in ra màn hình thường
để bạn kéo lên xem hoặc copy.

Màn hình này cần terminal đủ lớn (từ 70 cột và 16 dòng). Nhỏ hơn hoặc terminal không hỗ trợ thì `wj usage` quay về
kiểu gõ lệnh từng dòng như trước. Lần mở đầu tiên đọc cả tháng nên mất vài giây; mỗi lần bấm phím sau đó khoảng 0.1 giây.

Đầu báo cáo có bảng **Các kỳ**: ngày, tuần và tháng chứa kỳ đang xem (đang xem hôm nay thì là hôm nay, tuần này,
tháng này; lùi về 2026-10-20 thì là ngày đó, tuần W43, tháng 10), với số session, lượt, token và ≈USD; kỳ đang xem
đánh dấu `▶`. Bảng đi theo bộ lọc `project`/`app` đang chọn. Kỳ gõ tay không phải ngày/tuần/tháng (`7d`, một khoảng
ngày) tự thêm một dòng riêng.

Dưới bảng là **biểu đồ cột** token của các kỳ cùng kiểu quanh kỳ đang xem: 14 ngày, các tuần hay các tháng gần
nhất (rộng màn hình thì nhiều cột hơn), cột của kỳ đang xem có `▲` bên dưới. Các kỳ trống ở đầu (trước khi có phiên
nào trên máy) được bỏ bớt.

```text
Các kỳ (1 2 3 hoặc day/week/month để đổi; ← → kỳ trước/sau; ▶ là kỳ đang xem):
   KỲ       TÊN        NGÀY                     SESSION   LƯỢT    TỔNG  QUY ĐỔI
   1 day    hôm nay    2026-11-10 T3                  7   4787  536.7M     $319
▶  2 week   tuần này   2026-11-09 → 2026-11-15        9   9436    1.2B     $681
   3 month  tháng này  2026-11-01 → 2026-11-30       66  49524    8.7B   $3,826

Token theo tuần (← → xem tuần trước/sau; ▲ là kỳ đang xem):
 228.5M  176.6M   3.0B    1.9B    7.5B    1.2B
                                 ██████
                                 ██████
                                 ██████
         ▃▃▃▃▃▃                  ██████
         ██████  ▃▃▃▃▃▃          ██████
 ▁▁▁▁▁▁  ██████  ██████  ██████  ██████  ▇▇▇▇▇▇
  W41     W42     W43     W44     W45     W46
                                           ▲
```

Bảng và biểu đồ chỉ có ở màn hình `wj usage` (không kỳ, không cờ). `wj usage week` và các lệnh có kỳ in một lần như
cũ, không có chúng.

| Gõ ở dòng `>` | Tác dụng |
|---|---|
| `1` `2` `3`, `day` `week` `month` | xem theo ngày / tuần / tháng, giữ ngày đang xem (tuần bắt đầu thứ Hai) |
| `prev`, `next` (hoặc `<`, `>`) | kỳ trước / kỳ sau, như `←` `→` |
| `today`, `yesterday`, `lastweek`, `lastmonth` | tới thẳng kỳ đó |
| `2026-11-09`, `2026-W45`, `2026-11` | một ngày, một tuần (ISO), một tháng |
| `7d`, `30d` | N ngày gần nhất, tính cả hôm nay; `←` `→` lùi/tiến đúng N ngày |
| `2026-11-01..2026-11-09` | khoảng ngày bất kỳ (gồm cả hai đầu) |
| `agents` / `agents off` | bật/tắt liệt kê từng worker dưới session của nó |
| `open` / `open off` | chỉ các phiên Claude Code đang mở lúc này (xem mục "Phiên đang mở") |
| `top 30`, `top 0`, `top auto` | số session hiện ra (0 = tất cả, auto = vừa màn hình) |
| `project TÊN`, `project all` | lọc theo project |
| `app claude`, `app codex`, `app opencode`, `app antigravity`, `app all` | lọc theo app |
| `analytics` / `analytics off` | phân tích token của kỳ đang xem thay cho bảng session (xem mục "Phân tích"); ← → và day/week vẫn đổi kỳ |
| `blocks` / `blocks off` | khối 5 giờ thay cho bảng session (xem mục "Khối 5 giờ") |
| `limit 5M`, `limit max`, `limit off` | giới hạn token một khối |
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
- Dùng nhiều app (Claude, Codex, OpenCode, Antigravity) thì có thêm dòng `Theo app`.

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

Nền overlay gần đục (mặc định 92%) để chữ đọc rõ trên mọi nền, có viền mỏng. Chữ luôn đặc hoàn toàn.
Dòng tổng có chữ đậm, và phần mở rộng có dòng kẻ dưới tiêu đề; các model thụt vào dưới project.
Muốn thấy phần bên dưới thì chỉnh độ trong suốt (xem "Menu chuột phải").

Bấm đúp vào overlay (hoặc chuột phải, chọn Mở rộng) để xem báo cáo: app, lần dùng gần nhất,
model, token vào, ra, cache và số tiền quy đổi của từng project, thêm cả project đã đóng trong
khoảng đang xem và tổng cả máy. Project dùng nhiều model có thêm một dòng cho mỗi model; nếu có
quá nhiều dòng thì project đó chỉ ghi `3 model`, xem đủ bằng `wj usage -by project-model`.
Bấm đúp lần nữa để thu gọn. Mỗi lần mở overlay đều bắt đầu ở dạng thu gọn.

## Cảnh báo

```text
wj overlay -warn 5M
```

Dòng tổng hiện `16.5M/5M`, chuyển vàng từ 80% ngưỡng và đỏ khi vượt. Dưới dòng tổng có thanh
đo: xanh lá khi dưới 80%, vàng từ 80%, đỏ khi vượt ngưỡng. Thanh này chỉ hiện khi có `-warn`.
Ngưỡng do mình tự đặt, tính trên tổng cả máy trong khoảng đang xem (`-since`, hoặc đổi trong menu
chuột phải). Mặc định không cảnh báo.

Đây không phải limit thật của nhà cung cấp. Số token không cho biết còn lại bao nhiêu: gói Claude
xem bằng `/usage`, còn gateway xem trên trang của gateway.

## Menu chuột phải

Chuột phải vào overlay mở menu:

- **Mở rộng / Thu gọn (bấm đúp)**: như bấm đúp.
- **Khoảng thời gian**: 1 giờ, 24 giờ, 7 ngày. Đổi khoảng đang xem, không cần mở lại.
- **Độ trong suốt**: đặt độ đục của nền 40%, 60%, 80%, 92%, 100% (mặc định 92%; 100% là nền đặc, không thấy phần bên dưới).
- **Cỡ chữ**: 85%, 100%, 125%, 150% (mặc định 100%).
- **Đóng overlay**.

Kéo để đổi chỗ. Vị trí, khoảng thời gian, độ trong suốt và cỡ chữ được lưu trong `overlay.json`
cho lần mở sau. Overlay tự đổi cỡ theo DPI của màn hình đang đứng và luôn nằm trong vùng làm việc
của màn hình (không bị kéo ra khỏi màn hình). Hiện chỉ có trên Windows; macOS và Linux dùng
`wj top` trong một cửa sổ terminal nhỏ.

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
- **Cursor chưa đo được:** Cursor ghi token trên máy chủ của họ chứ không trong file trên máy, nên không có
  mặt trong `wj usage` và `wj top`. Báo cáo chỉ gồm các app ghi trong dòng đầu.
- App khác đo bằng `wj proxy run` và `wj proxy usage`. Xem `wj help proxy`.

## Phiên đang mở

Mỗi dòng session trong báo cáo có dấu sau mã nếu phiên đó **đang mở**: `●` là đang trả lời, `○` là đang rảnh
(chờ bạn). Không có dấu nghĩa là phiên đã đóng. Gõ `open` ở dòng lệnh để chỉ xem các phiên đang mở, kèm token của
chúng trong kỳ đang chọn; gõ `open off` để quay về mọi session. Chế độ `open` bỏ bảng "Theo ngày" và cây model, vì
tổng chỉ tính cho các phiên đang mở.

- **Nguồn:** Claude Code tự ghi một file nhỏ cho mỗi phiên đang chạy ở `~/.claude/sessions/<pid>.json`
  (pid, session id, thư mục làm việc, bận hay rảnh). `wj` đọc các file đó và kiểm tra tiến trình còn sống; trên
  Windows còn so giờ bắt đầu tiến trình để không nhầm với một tiến trình khác dùng lại cùng pid.
- **Tự cập nhật:** dấu `●`/`○` đổi khi một phiên mở, đóng hoặc chuyển giữa bận và rảnh, và màn hình tự vẽ lại (chu
  kỳ 5 giây như các số khác).
- **Phiên nền (`bg`)** như worker chạy ngầm cũng được tính là đang mở.
- **Giới hạn:** chỉ Claude Code. Codex và OpenCode không ghi file nào cho phiên đang chạy, nên `wj` chưa nhận ra
  phiên đang mở của chúng; dòng `open` chỉ hiện các phiên Claude.

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

## Antigravity

Antigravity (IDE và CLI `agy`) ghi mỗi cuộc trò chuyện vào một file SQLite
`~/.gemini/antigravity*/conversations/<id>.db` (các thư mục `antigravity`, `antigravity-cli`, `antigravity-ide`...;
đổi bằng `ANTIGRAVITY_DATA_DIR`, nhiều thư mục cách nhau dấu phẩy). Số token nằm trong các khối protobuf không có tài
liệu; `wj` đọc theo số trường mà [ccusage](https://github.com/ccusage/ccusage) đã dò ra, chỉ đọc token, model, thời
gian, thư mục làm việc và tên/việc của agent phụ, không đọc nội dung.

- **Một lượt gọi tính một lần,** dù Antigravity ghi nó ở cả bước (`steps`) và lượt sinh (`gen_metadata`), hay ở cả cuộc
  trò chuyện cha và con.
- **Agent phụ tính vào cuộc trò chuyện cha,** như worker của Claude: xem bằng `agents`.
- **Tên model** như "Gemini 3.8 Flash (High)" được đổi về tên trong bảng giá; mức suy nghĩ (high/medium/low) tính
  theo giá của model gốc. Model chỉ có mã số mà chưa biết tên (`model_placeholder_m322`) thì không có giá (dấu `?`).

## Thư mục Claude

Mặc định đọc `~/.claude` và cả `~/.config/claude` (hoặc `$XDG_CONFIG_HOME/claude`) nếu có thư mục `projects` ở đó.
`CLAUDE_CONFIG_DIR` nhận nhiều thư mục cách nhau dấu phẩy (ví dụ hai tài khoản); đặt biến này thì chỉ đọc các thư mục đó.

## Khối 5 giờ

`wj usage blocks` (hoặc gõ `blocks` trong màn hình tương tác) chia lượt gọi thành các khối 5 giờ, theo cách gói Claude tính
hạn mức: khối bắt đầu ở giờ tròn của lượt gọi đầu tiên và dài 5 giờ; lượt gọi quá 5 giờ sau khi khối bắt đầu, hoặc sau
lượt gọi trước đó, mở khối mới. Nghỉ quá 5 giờ thì có một dòng `nghỉ`. Cũng nhận kỳ: `wj usage blocks week`.

```text
   BẮT ĐẦU  GỌI CUỐI  LƯỢT    TỔNG  VÀO+RA  QUY ĐỔI  MODEL          % GIỚI HẠN
   12:00    16:58     1449  158.3M    1.6M   $94.18  opus-5-5 +1           83%
▶  22:00    22:33      297   33.4M  580.5k   $19.77  opus-5-5 +1           17%

▶ Khối đang chạy: 22:00 → 03:00, đã 33 phút, còn 4 giờ
  Tốc độ: 999.0k token/phút (vào+ra 17.4k/phút), $35.50/giờ
  Dự kiến hết khối: 299.5M token, $177 — 157% giới hạn 190.9M: VƯỢT
```

- **Tốc độ** = token từ lượt đầu tới lượt cuối của khối, chia số phút giữa hai lượt; `vào+ra` là phần không cache.
- **Dự kiến** = số hiện tại cộng tốc độ × số phút còn lại tới hết khối. Chỉ có khi khối đang chạy.
- **`-limit 5M`** (hoặc `limit 5M` ở dòng lệnh) thêm cột % GIỚI HẠN và trạng thái: ổn, SẮP CHẠM (từ 80%), VƯỢT.
  `-limit max` lấy khối lớn nhất đã xong làm giới hạn. Giới hạn tính trên TỔNG (cả cache).
- Khối tính cho mọi app; lọc bằng `-app claude` để xem riêng Claude.

## Phân tích

`wj usage analytics [kỳ]` (hoặc gõ `analytics` trong màn hình tương tác, rồi day/week/←/→ để đổi ngày, tuần) trả lời câu
"vì sao tốn token thế". Mỗi lượt gọi, model đọc lại cả cuộc trò chuyện, nên phần lớn token là ngữ cảnh đọc lại; báo cáo
chia số token và ≈USD của kỳ theo:

1. **Loại token**: đọc cache, ghi cache, vào, ra, và phần suy nghĩ (thinking/reasoning) trong ra. Đọc cache thường
   chiếm hơn 90% token nhưng ít tiền; ghi cache và ra thường mới là phần đắt.
2. **Cỡ ngữ cảnh mỗi lượt**: dưới 20k, 20k-50k, ... trên 500k. Lượt ngữ cảnh to đắt hơn hẳn.
3. **Tool**: lượt đó model gọi tool gì (Read, Bash, Edit, exec_command...). Kết quả tool vào ngữ cảnh và bị đọc lại ở
   mọi lượt sau. Lượt gọi nhiều tool chia đều cho các tool.
4. **Ngữ cảnh bị làm lại**: cache hết hạn (nghỉ quá 5 phút, hoặc 1 giờ với cache 1 giờ) phải ghi lại cả ngữ cảnh; và số
   lần nén ngữ cảnh (compact) cùng cỡ ngữ cảnh lúc nén.
5. **Ai tiêu**: phiên chính hay worker (subagent), theo model.
6. **Giờ trong ngày**: biểu đồ 00-23.
7. **Session tốn nhất**: ngữ cảnh trung bình, lớn nhất, số lần nén, số lần cache lại, tool dùng nhiều nhất.

Cuối cùng là **Vì sao tốn**: vài câu rút ra từ số liệu trên (ví dụ worker chiếm 80% token, Bash chiếm 80% token,
model opus chiếm 79% chi phí) và nên làm gì.

Tool, suy nghĩ và nén có cho Claude Code và Codex; OpenCode và Antigravity chỉ có phần loại token, ngữ cảnh, model, giờ.
Lần nén của Codex không có cỡ sau khi nén; cỡ trước là ngữ cảnh của lượt cuối trước đó.

## JSON

Thêm `-json` vào lệnh in một lần để có JSON thay vì bảng, cho script hoặc status line:

- `wj usage week -json`: `period`, `totals`, `days`, `sessions` (kèm `workers`) và `models` (kèm token theo project).
- `wj usage blocks -json -limit 5M`: `blocks` với `burnRate` và `projection` (`limitStatus`: ok, warning, exceeds).
- `wj usage analytics week -json`: `byKind`, `byContextSize`, `byTool`, `cacheRebuilds`, `compactions`,
  `mainThreads`/`subagents`, `tokensByHour`, `sessions`, `insights`.
- `wj usage -since 24h -by model -json`: `totals` và `groups`.

Token là số nguyên, `costUSD` theo giá API niêm yết như cột QUY ĐỔI, thời gian theo RFC 3339.
