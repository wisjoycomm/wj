package main

import (
	"fmt"
	"io"
	"strings"
)

const helpMain = `wj - cài và kết nối công cụ lập trình AI cho máy mới

CÁCH DÙNG NHANH
  wj                 mở menu, chọn bằng số (dễ nhất)
  wj help CHỦ_ĐỀ     xem hướng dẫn chi tiết một chủ đề

CHỦ ĐỀ HƯỚNG DẪN
  help claude    kết nối Claude Code (gateway, token OAuth, API key, đăng nhập web)
  help codex     kết nối Codex CLI / OpenCode
  help usage     xem token mọi project: wj usage, wj top, wj overlay
  help models    lấy model từ gateway, Claude Code/Codex dùng chéo model được không
  help proxy     đo token bằng mitmproxy (cho app khác Claude/Codex)
  help apps      cài / cập nhật ứng dụng
  help tools     bộ công cụ dòng lệnh, quản lý cửa sổ
  help init      setup ban đầu cho một project (rule chung, rule theo job, skill Orca)
  help check     xem các agent có khớp cấu hình không
  help words    giải thích các từ hay gặp (gateway, token, profile...)

LỆNH HAY DÙNG
  wj apps list                   xem app nào đã cài
  wj apps install claude,codex   cài app còn thiếu
  wj configure claude            kết nối Claude Code
  wj configure codex             kết nối Codex
  wj -use-profile adnx -run      đổi sang kết nối "adnx" và mở Claude
  wj top                         token mọi project, cập nhật liên tục
  wj overlay                     token luôn nổi trên màn hình (Windows)
  wj usage                       token Claude, Codex, OpenCode, Antigravity theo ngày/tuần/tháng
  wj models                      lấy lại danh sách model từ gateway
  wj proxy run claude            mở Claude qua mitmproxy (tự cài, tự bật)
  wj direct                      Claude gọi thẳng gateway, bỏ proxy đo token
  wj init                        setup ban đầu cho project đang đứng
  wj check                       so cấu hình giữa các agent (chỉ đọc)
  wj doctor                      tự kiểm tra máy, báo vấn đề, sửa (-dry-run chỉ báo, -ai nhờ AI gợi ý)
  wj plan                        PLAN.md một bảng: task sẵn sàng, check format, convert kiểu cũ
  wj verify T-01                 worker: kiểm tra một task trong worktree
  wj digest                      lỗi lặp lại trong các phiên Claude Code của project (cho /orca-dream)
  wj guard                       hook của Claude Code: chặn lượt đọc tốn token (wj init tự gắn)
  wj trash <file>...             chuyển file vào thùng rác thay vì xoá hẳn (agent dùng thay rm)
  wj update                      cập nhật chính tool này
  wj version                     xem phiên bản

Thêm -dry-run vào hầu hết lệnh để xem trước, không thay đổi gì.`

const helpClaude = `KẾT NỐI CLAUDE CODE

  wj configure claude    (hoặc: wj auth) - hỏi từng bước

Bạn đang có gì?                                    Chọn
  Domain công ty (vd ai.adnx.vn) + key của domain   1. Gateway
  Token bắt đầu bằng sk-ant-oat...                  2. Token OAuth
  API key từ console.anthropic.com (sk-ant-api...)  3. API key
  Tài khoản Claude Pro/Max, đăng nhập trên web      4. Đăng nhập web

Lệnh có sẵn tham số (không phải trả lời câu hỏi):
  Gateway:   wj auth -auth gateway -only claude -base-url ai.adnx.vn -profile adnx
  OAuth:     wj auth -auth oauth -profile work
  API key:   wj auth -auth api-key -profile api
  Web:       wj auth -auth login

Lưu & đổi kết nối (profile):
  -profile TÊN          lưu kết nối vừa nhập với tên này
  -list-profiles        xem các kết nối đã lưu (không hiện key)
  -use-profile TÊN      chuyển sang kết nối đã lưu
  -run                  mở Claude ngay sau khi xong

Tham số khác:
  -install-claude       tự cài Claude Code nếu máy chưa có
  -auth-header x-api-key  khi gateway đòi header x-api-key thay vì Bearer
  -skip-check -models ID1,ID2   gateway không có /v1/models: tự nhập tên model
  -key-stdin            đọc key từ stdin (cho script), không hiện ra màn hình
  -no-login             (đăng nhập web) chỉ dọn cấu hình, chưa mở trình duyệt
  -neutral-headers      (gateway) Claude Code gửi header trung tính thay vì danh tính Claude Code.
                        Chỉ dùng khi gateway từ chối Claude Code (lỗi 400 reasoning_effort).
                        Được lưu trong profile; tắt bằng -neutral-headers=false
  -dry-run              xem trước, không ghi file, không gọi mạng

Model qua gateway: tool lấy danh sách model của gateway, gán Opus/Sonnet/Haiku/Fable
bản mới nhất, bật CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY để Claude tự cập nhật
model Claude, và thêm model khác (GPT...) vào /model. Mỗi lần -use-profile, danh sách
được lấy lại và lưu vào profile. Xem thêm: wj help models.

Sau khi đổi: mở phiên Claude MỚI và gõ /status để kiểm tra.
Tool giữ nguyên hooks, plugin, statusLine; file cũ được sao lưu .bak-<thời gian>.

Lỗi hay gặp: dán token sk-ant-oat vào mục Gateway -> báo 401. Token OAuth phải
chọn mục 2. Cookie sessionKey (sk-ant-sid...) của trình duyệt KHÔNG dùng được.`

const helpCodex = `KẾT NỐI CODEX CLI / OPENCODE

  wj configure codex      hỏi từng bước
  wj configure opencode

Codex có 3 cách:
  1. Đăng nhập ChatGPT     wj configure codex -auth login
                           (máy không mở được trình duyệt: thêm -device-auth)
  2. API key OpenAI        wj configure codex -auth api-key
  3. Gateway công ty       wj configure codex -auth gateway -base-url ai.adnx.vn
     Dùng lại gateway đã lưu cho Claude (khỏi nhập key lần nữa):
                           wj configure codex -from-profile adnx

  Gateway cho Codex phải hỗ trợ /v1/responses. Tool lấy TẤT CẢ model của gateway
  đưa vào /model của Codex (GPT mới nhất đứng đầu, làm mặc định). Chạy lại lệnh
  bất cứ lúc nào để cập nhật khi gateway thêm model.
  Chỉ muốn một model: -models cx/gpt-6.1-sol
  Gateway không có /v1/models: thêm -skip-check -models TÊN_MODEL

  Cấu hình ghi vào ~/.codex/config.toml và ~/.codex/wj-models.json
  (giữ projects, MCP, hooks; có sao lưu).
  Token Claude (sk-ant-...) KHÔNG dùng cho Codex.

OpenCode:
  -auth login                          đăng nhập trong OpenCode
  -auth api-key -provider-id openai    key OpenAI trực tiếp
  -auth api-key -provider-id anthropic key Anthropic trực tiếp
  -auth gateway -base-url DOMAIN       gateway công ty (lấy đủ model của gateway)
  -from-profile adnx                   dùng lại gateway đã lưu cho Claude

Cursor, Antigravity, Orca tự đăng nhập trong app; configure chỉ in hướng dẫn.`

const helpApps = `CÀI / CẬP NHẬT ỨNG DỤNG

  wj apps list                     app nào đã cài, app nào thiếu (không cần mạng)
  wj apps install claude,codex     cài app còn thiếu (bỏ qua app đã có)
  wj apps install all -dry-run     xem sẽ cài gì
  wj apps update codex,orca        cập nhật (chỉ khi bạn gọi lệnh này)

Tên app: claude, codex, opencode, cursor-agent, cursor, antigravity (agy),
         antigravity-ide, orca, mitmproxy

Windows cần WinGet cho opencode/cursor/antigravity-ide/mitmproxy; macOS cần Homebrew.
Cài xong mà gõ lệnh không thấy: đóng terminal, mở cái mới.`

const helpTools = `BỘ CÔNG CỤ DÒNG LỆNH & QUẢN LÝ CỬA SỔ

  wj tools list                      xem có những nhóm nào
  wj tools install cli               cài (hỏi chọn nhóm)
  wj tools install cli -groups 1,2,4 chọn sẵn nhóm
  wj tools install wm                quản lý cửa sổ kiểu i3
  wj tools uninstall cli | wm        gỡ

Nhóm: 1 terminal & giao diện   2 tiện ích (rg, fd, bat, lazygit...)
      3 git, node, python, neovim   4 Claude Code, Codex, Antigravity CLI`

const helpCheck = `XEM CÁC AGENT CÓ KHỚP CẤU HÌNH KHÔNG

  wj check    chỉ đọc, không ghi gì, không cần mạng

Với Claude, Codex, Antigravity, Cursor, OpenCode, lệnh cho biết: đã cài chưa, đang
kết nối kiểu gì, có file chỉ dẫn chung chưa, có MCP server và skill nào. Sau đó liệt
kê thứ mà agent này có còn agent khác đã cài thì thiếu.

Chỉ hiện tên, không bao giờ hiện key: giá trị trong file cấu hình không được in ra.`

const helpUsage = `XEM TOKEN MỌI PROJECT

Đọc thẳng file phiên của Claude Code, Codex, OpenCode và Antigravity trên máy này. Không cần proxy, không
gọi mạng, không ghi gì; tính cả phiên mở từ Orca. Các worktree của một repo gộp về
một project.

  wj top                  bảng trực tiếp như overlay mở rộng: app, model (mỗi model một dòng), lượt,
                          vào, ra, đọc/ghi cache, tổng, ≈USD; project vừa chạy lên đầu, có tổng cả máy
                          theo 24 giờ, 7 ngày, 30 ngày, toàn bộ (Ctrl+C để thoát)
  wj top -once            in một lần
  wj overlay              cửa sổ nhỏ luôn nổi: mọi project đang mở/đang chạy và tổng cả máy;
                          project mới tự hiện; bấm đúp để xem chi tiết, kéo để đổi chỗ,
                          chuột phải để đóng
  wj overlay -warn 5M     dòng tổng hiện 16.5M/5M, chuyển vàng từ 80% ngưỡng, đỏ khi vượt
  wj overlay stop         đóng overlay (overlay hiện chỉ có trên Windows)
  wj usage                màn hình tương tác: báo cáo ở trên, danh sách lệnh (dropdown) và dòng nhập ở dưới.
                          Kỳ: day, week, month (1 2 3); ← → kỳ trước/sau; ↑↓ cuộn. Tab vào danh sách lệnh,
                          ở đó ↑↓ chọn → báo cáo đổi theo ngay; Enter chốt; gõ để lọc; Esc rời; q hoặc Ctrl+C
                          thoát. Lệnh: day, week, month, analytics, agents, open, blocks, limit, top, project, app, reset, help.
                          Đầu báo cáo có bảng ngày/tuần/tháng và biểu đồ token theo kỳ; cuối có cây tổng hợp.
                          Phiên Claude đang mở có dấu ● (đang trả lời) hoặc ○ (rảnh); gõ open để chỉ xem chúng
  wj usage week           không có terminal (pipe, script) hoặc có kỳ/cờ: in một lần rồi thoát
                          kỳ: today, yesterday, week, lastweek, month, lastmonth, 7d (N ngày gần nhất),
                          2026-11-09, 2026-W45, 2026-11, 2026-11-01..2026-11-09. Tuần bắt đầu thứ Hai
  wj usage month -top 0   -top N số session hiện ra (mặc định tự vừa màn hình / 20, 0 = tất cả)
  wj usage today -agents  liệt kê từng worker (subagent) dưới session của nó: mô tả việc, model, lượt, token, ≈USD
  wj usage lastweek -project game -app claude    lọc theo project, app (claude, codex, opencode, antigravity)
  wj usage blocks         khối 5 giờ như gói Claude tính: mỗi khối lượt, token, ≈USD; khối đang chạy có tốc độ
                          (token/phút, ≈USD/giờ) và dự kiến lúc hết khối. Cũng nhận kỳ: wj usage blocks week
  wj usage blocks -limit 5M   giới hạn token một khối: cột % và cảnh báo từ 80%; -limit max = khối lớn nhất trước đó
  wj usage analytics week phân tích vì sao tốn token: loại token (đọc/ghi cache, ra, suy nghĩ), cỡ ngữ cảnh,
                          tool mỗi lượt gọi, cache hết hạn, nén ngữ cảnh, phiên chính/worker, model, giờ trong ngày,
                          session tốn nhất và vài câu "Vì sao tốn". Kỳ như trên; không ghi kỳ = hôm nay
  wj usage week -json     in JSON thay vì bảng (cho script, status line); cũng dùng với blocks, analytics và -since/-by
  wj usage -since 24h     bảng cũ theo project + app, có cột quy đổi USD theo giá API
  wj usage -by project-model   từng model trong mỗi project
  wj usage -by app        theo app + model; còn có -by project, -by model
  wj usage -project game -app codex
  -since 168h             khoảng thời gian (mặc định 24h); wj usage còn có -all

Thư mục: CLAUDE_CONFIG_DIR (nhiều thư mục cách nhau dấu phẩy; mặc định ~/.claude và ~/.config/claude),
CODEX_HOME, OPENCODE_DATA_DIR, ANTIGRAVITY_DATA_DIR (mặc định ~/.gemini/antigravity*).
Chỉ tính được khi file phiên còn trên máy. Chưa có: Cursor (token nằm trên máy chủ của họ). App khác: wj help proxy.`

const helpModels = `MODEL TỪ GATEWAY

  wj models        lấy lại danh sách và cập nhật mọi app đang nối gateway, dùng key
                   đã lưu trong từng app; giữ nguyên model mặc định
  wj models test   gửi một câu ngắn có tool tới từng model, báo app nào dùng được
                   (tốn vài token, hỏi trước; thêm -models ID1,ID2 để thử vài model)

Tên model giữ nguyên theo danh sách gateway trả về; mỗi gateway đặt tên một kiểu.

Tự động: wj configure, wj auth và -use-profile luôn lấy danh sách mới. Model Claude
trong Claude Code còn tự cập nhật mỗi lần mở app. Model khác trong Claude Code và danh
sách của Codex thì cập nhật khi chạy một trong các lệnh trên.

Dùng chéo model:
  Claude Code chỉ nói /v1/messages, Codex chỉ nói /v1/responses. App chọn được model
  khác hãng, nhưng gateway phải tự chuyển đổi đường đó. Hãng không hỗ trợ chính thức
  việc này: có thể lỗi ở tool, suy luận hoặc nén ngữ cảnh. Kiểm tra bằng wj models test.`

const helpInit = `SETUP BAN ĐẦU CHO MỘT PROJECT

  cd vào project, rồi:
  wj init                     hỏi loại job (tự phát hiện sẵn), cài rule + skill Orca
  wj init -type unity         chọn sẵn; nhiều loại thì: -type unity,web
  wj init -dry-run            xem trước, không ghi gì
  wj init -update             project cài bằng kit cũ: lấy skill mới, bỏ script .ps1 và skill worker cũ
  wj init -lite               chỉ phần không cần wj: rule, quyền, file cấm đọc (không skill Orca, không hook).
                              Người không cài wj lấy đúng phần này từ wj-kit-lite-<job>.zip trong release

Loại job: unity, web, go, python. Luôn có rule chung cho mọi agent.

Tool cài (cho Claude Code):
  AGENTS.md              rule chung + rule theo job
  CLAUDE.md, PLAN.md     ghi chú riêng cho Claude Code, kế hoạch task
  .claude/               skill /orca-orchestrator và /orca-dream, subagent orca-worker,
                         quyền chạy lệnh, file cấm đọc (lockfile, cache, .env),
                         hook wj guard chặn đọc cả file dài, và giới hạn output Bash
  .claude/settings.local.json   memory của Claude Code nằm trong project (.claude/memory/),
                         không còn ở ~/.claude. File này không vào git: mỗi worktree chạy wj init một lần
  .gitignore             .tmp/, memory, worktree của worker, thư mục build của job

wj không bắt buộc: thiếu wj thì skill và worker làm tay các bước wj plan, wj verify (tốn token
hơn) và hook wj guard tự bỏ qua. Riêng /orca-dream cần wj digest.
Cách dùng: /orca-orchestrator <mục tiêu> để chia task, rồi /orca-orchestrator go để chạy
worker (subagent của Claude Code, mỗi worker một worktree). Chi tiết: docs/project.md.

An toàn khi chạy lại:
  - File đã có thì giữ nguyên, kể cả skill bạn đã sửa; tool chỉ báo là khác bản kit.
    Riêng skill Orca: -update thay bằng bản kit, bản cũ sao lưu .bak-<giờ>.
  - AGENTS.md: chỉ cập nhật phần nằm giữa dấu wj:begin / wj:end, phần bạn tự viết giữ nguyên.
  - settings.json và .gitignore: chỉ thêm, không xóa; lệnh bạn đã chặn vẫn bị chặn.

Xong thì commit các file này: worker Orca chỉ thấy file đã commit.`

const helpWords = `GIẢI THÍCH TỪ NGỮ

  Gateway / custom domain  Máy chủ trung gian của công ty (vd ai.adnx.vn) cấp key riêng
                           và chuyển tiếp tới nhiều model (Claude, GPT, DeepSeek...).
  Token OAuth (sk-ant-oat) Token dài hạn của tài khoản Claude Pro/Max, thường do
                           admin cấp hoặc tạo bằng "claude setup-token".
  API key                  Key trả tiền theo lượng dùng, từ console.anthropic.com
                           hoặc platform.openai.com.
  Profile                  Một kết nối Claude đã lưu có tên (vd adnx, work) để đổi nhanh.
  Model ID                 Tên model gateway dùng, vd cc/claude-opus-5-5, cx/gpt-6.1-sol.
  Proxy / mitmproxy        Chương trình đứng giữa app và gateway để đếm token đã dùng.
  Dry run (-dry-run)       Chạy thử: chỉ in ra sẽ làm gì, không thay đổi máy.`

func printHelp(args []string, input io.Reader, out io.Writer) error {
	topic := ""
	if len(args) > 0 {
		topic = strings.ToLower(args[0])
	}
	switch topic {
	case "":
		fmt.Fprintln(out, helpMain)
	case "claude", "auth":
		fmt.Fprintln(out, helpClaude)
	case "codex", "opencode", "configure":
		fmt.Fprintln(out, helpCodex)
	case "proxy", "mitmproxy":
		fmt.Fprintln(out, proxyHelp)
	case "apps", "app":
		fmt.Fprintln(out, helpApps)
	case "tools", "tool":
		fmt.Fprintln(out, helpTools)
	case "usage", "top", "overlay":
		_, err := fmt.Fprintln(out, helpUsage)
		return err
	case "models", "model":
		_, err := fmt.Fprintln(out, helpModels)
		return err
	case "init":
		fmt.Fprintln(out, helpInit)
	case "check":
		fmt.Fprintln(out, helpCheck)
	case "doctor":
		fmt.Fprintln(out, doctorHelp)
	case "plan":
		fmt.Fprintln(out, planHelp)
	case "verify":
		fmt.Fprintln(out, verifyHelp)
	case "trash":
		fmt.Fprintln(out, trashHelp)
	case "guard":
		fmt.Fprintln(out, guardHelp)
	case "digest", "dream":
		fmt.Fprintln(out, digestHelp)
	case "words", "terms", "tu":
		fmt.Fprintln(out, helpWords)
	case "flags", "all":
		fmt.Fprintln(out, "Tất cả tham số kết nối Claude:")
		return runCLI([]string{"-help"}, input, out)
	default:
		return fmt.Errorf("không có chủ đề %q; gõ wj help để xem danh sách", topic)
	}
	return nil
}
