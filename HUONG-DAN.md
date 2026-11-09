# Huong dan cai may moi - Wisjoy wj

Ten cu la `setup-gateway`. May da cai van chay binh thuong: lan chay dau tien tool tu
chuyen `~/.setup-gateway` sang `~/.wj` va tao them lenh `wj` canh lenh cu.

Setup ban dau cho mot project: `cd` vao project roi chay `wj init` (chon job: unity,
web, go, python; them `-dry-run` de xem truoc). Tool cai rule chung, rule theo job va
skill Orca, khong de file da co. Chi tiet: `wj help init`.

De xem cac agent co khop cau hinh khong, chay `wj check` (chi doc, khong can
mang, khong hien key).

Xem token moi project: `wj top`, `wj overlay`, `wj usage`. Lay lai model tu gateway:
`wj models`. Tai lieu ngan cho tung phan: [docs/](docs/README.md).

Mot tool CLI gom bo `Bo_cong_cu`, Claude, Codex, OpenCode, Cursor, Antigravity va Orca.
Ban tai mot file executable, khong can cai Go hay Node de mo tool.

## 1. Cach de nhat tren Windows

Mo **CMD**, dan mot lenh nay:

```bat
powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm 'https://github.com/wisjoycomm/wj/releases/latest/download/install.ps1')))"
```

Tool tu tai ban release, kiem tra SHA-256 va cai vao PATH. Cai xong KHONG tu mo: ai can thi go `wj` (muon mo ngay thi them `-RunSetup` vao cuoi lenh).
Khong can tu tai ZIP hay giai nen. Lan sau mo CMD moi va go `wj`.
Neu CMD/Windows Terminal dang mo tu truoc, dong han va mo lai de nhan PATH moi.

Neu muon chuyen file sang may khac, van co cach dung ZIP:

1. Vao muc **Releases** cua repo va tai `wj-windows-amd64.zip`.
2. Giai nen ra mot thu muc.
3. Chay `Run-Setup.cmd`.
4. Chon `1` de cai may moi, hoac chon tung muc rieng.

Neu chua phat hanh ban moi: dung ZIP trong thu muc `dist` cua nguoi build.
Neu chi co source Git, xem muc Build o cuoi file nay.

Menu:

```text
   1. Ứng dụng & môi trường (máy mới, ứng dụng, công cụ dòng lệnh)
   2. Setup project (rule, skill Orca)
   3. AI Agent (kết nối, model, xem token, kiểm tra cấu hình)
   4. Cập nhật wj

   h. Hướng dẫn nhanh      0. Thoát
```

Quen lenh nao thi go `wj help` (hoac `help claude`, `help codex`,
`help proxy`, `help apps`, `help tools`, `help words` de xem giai thich tu ngu).
Tren Windows tool tu bat UTF-8 de hien dung tieng Viet co dau.

Che do may moi hoi tung phan. Khong can cai het. Phan quan ly cua so la tuy chon.
Khi cai package, Windows co the hien UAC; Homebrew/Linux co the hoi mat khau sudo.
Menu quay lai sau moi thao tac, ke ca khi gap loi; nhan 0 de thoat.
Moi danh sach (app, model, profile...) deu danh so: go so la chon, nhieu so cach
nhau dau cach (vd `1 2 9`), khoang `2-4`, `all` = tat ca, Enter = mac dinh/quay lai.

### Cai mot lan, dung lai tai may

Sau lan cai dau, chi can go `wj`. Khong clone Git, khong tai lai
source va khong tu check update khi mo menu. Lenh `apps list` va `version`
cung hoat dong offline. Chi tai khi ban chon cai/cap nhat; dang nhap va
kiem tra gateway van can mang. Tung app co the co auto-update rieng.

```text
wj apps list
wj apps install claude,codex,opencode,cursor-agent,cursor,antigravity,antigravity-ide,orca,mitmproxy
wj apps install all -dry-run
wj apps update codex,orca
wj update
wj version
```

`apps install` bo qua app da phat hien tren may. `apps update` moi yeu cau cap nhat.
`update` chi cap nhat tool wj, sau do mo lai tool.

| ID | Windows | macOS / Linux |
| --- | --- | --- |
| claude | Installer chinh thuc | Installer chinh thuc |
| codex | Installer chinh thuc | Installer chinh thuc |
| opencode | WinGet SST.opencode | Installer chinh thuc |
| cursor-agent | Cursor CLI chinh thuc | Cursor CLI chinh thuc |
| cursor | WinGet Anysphere.Cursor | macOS: Homebrew; Linux: tai tu cursor.com/downloads |
| antigravity (hoac agy) | CLI chinh thuc | CLI chinh thuc |
| antigravity-ide | WinGet Google.AntigravityIDE | macOS: Homebrew; Linux: antigravity.google/download |
| orca | Installer tu stablyai/orca | macOS: ~/Applications/Orca.app; Linux: ~/.local/bin/orca-ide |
| mitmproxy | WinGet mitmproxy.mitmproxy | macOS: Homebrew; Linux: `uv tool install mitmproxy` |

Windows can App Installer/WinGet cho cac goi WinGet. macOS can Homebrew cho
ung dung cask. Wizard cho cai bo cong cu truoc de bo sung cac tien ich can thiet.
Linux: `all` van cai cac app ho tro, nhung bao loi rieng cho hai IDE can tai tay.
Orca AppImage can FUSE; neu thieu, thu `orca-ide --appimage-extract-and-run`.

Orca lay tu upstream chinh thuc `stablyai/orca`; fork `wisjoycomm/orca` chua co
release bo cai. File release Orca/wj duoc kiem tra SHA-256 va cache
trong thu muc user-cache cua he dieu hanh, muc `wj/downloads`.
Thu lai se dung file cache neu hash dung. Installer cac hang/WinGet/Homebrew
tu quan ly download/cache cua chung. Ban bootstrap Windows cung khong tai lai
binary neu may da co dung hash cua release.

### Dang nhap theo tung ung dung

```text
wj configure claude
wj configure codex
wj configure opencode
wj configure cursor
wj configure orca
```

Claude van co OAuth `sk-ant-oat`, API key, gateway, login. Token Claude khong
dung thay API key cua Codex/OpenCode. Codex co cac cach sau:

```text
wj configure codex -auth login
wj configure codex -auth login -device-auth
wj configure codex -auth api-key
wj configure codex -auth gateway -base-url https://example.com -models MODEL_THAT
```

Nhap key o prompt an. API key OpenAI di vao `codex login --with-api-key` qua stdin.
Gateway Codex phai ho tro `/v1/responses`; thay `MODEL_THAT` bang ID gateway cap.
Gateway: tool lay TAT CA model cua gateway dua vao `/model` cua Codex (GPT moi
nhat dung dau, lam mac dinh), ghi o `~/.codex/wj-models.json`. Chay lai
de cap nhat khi gateway them model. Dung lai gateway da luu cho Claude, khoi nhap key:
`wj configure codex -from-profile adnx` (OpenCode cung vay).
Claude qua gateway: tool gan Opus/Sonnet/Haiku/Fable ban moi nhat va bat
`CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY` de `/model` hien du model gateway.

Chuyen login se bo lua chon model/provider cu trong config user, giu project/MCP
va cau hinh khac. File TOML duoc backup, comment/dinh dang co the thay doi.
Bien OPENAI_BASE_URL con trong terminal hoac cau hinh project/managed van co the
anh huong khi ban tu mo Codex ben ngoai tool.

OpenCode:

```text
wj configure opencode -auth login
wj configure opencode -auth api-key -provider-id openai
wj configure opencode -auth api-key -provider-id anthropic
wj configure opencode -auth gateway -base-url https://example.com -models MODEL_THAT
```

Neu gateway khong co `/v1/models`, them `-skip-check -models MODEL_THAT`.
OpenCode dung schema `provider/options/npm` hien tai; giu cau hinh JSON/JSONC
khac va backup file duoc sua. Sau login, dung `/models` de chon provider muon dung.
Gateway cu van duoc giu lai trong danh sach.

Cursor/Antigravity dang nhap bang UI/CLI cua chinh ung dung; muc configure hien
huong dan. Cursor gateway vao Settings > Models neu tinh nang do ho tro, tool
khong ghi bien OPENAI_* toan may. Orca: mo app, them thu muc project, chon Claude
hoac Codex da cai/cau hinh; khong co key gateway dung chung rieng cho Orca.

## 2. Cai lenh vao PATH

Sau khi ban moi da duoc phat hanh, mo PowerShell:

```powershell
& ([scriptblock]::Create((irm 'https://github.com/wisjoycomm/wj/releases/latest/download/install.ps1')))
```

Installer tu chon ban Windows x64/ARM64 va kiem tra SHA-256.
File duoc cai vao `%USERPROFILE%\.local\bin\wj.exe`.
Lenh tren tai ban **release da phat hanh**, khong tu build source tren nhanh Git.

macOS / Linux:

```bash
curl -fsSL https://github.com/wisjoycomm/wj/releases/latest/download/install.sh | sh
~/.local/bin/wj
```

## 3. Cai bo cong cu cua ban

```powershell
wj tools list
wj tools install cli
```

| Nhom | Cong cu |
| --- | --- |
| 1 | Terminal, Starship, Nerd Font, fzf, zoxide; Windows co PowerShell 7, Windows Terminal, Clink |
| 2 | rg, fd, bat, eza, jq, lazygit, delta, btop, gh |
| 3 | Git, fnm/Node, uv/Python, Neovim, Antigravity IDE; Docker hoi rieng |
| 4 | Claude Code, Codex CLI, Antigravity CLI |

Chon san nhom:

```powershell
wj tools install cli -groups 1,2,4
```

Tool bo qua ung dung da co, them khoi cau hinh shell co danh dau, alias, prompt,
font va mot so mac dinh Git. Mo terminal moi sau khi cai de nhan PATH/profile.

Quan ly cua so:

```powershell
wj tools install wm
```

- Windows: Komorebi + whkd, phim Alt kieu i3, workspace theo man hinh, thanh bar.
- macOS: AeroSpace va phim tat tuong ung.
- Linux: cau hinh i3/Hyprland **da cai san**; module khong cai desktop session.

Phan nay thay doi shortcut, startup, taskbar/Dock va cach xep cua so.
Neu dang dung GlazeWM, tool hoi rieng truoc khi go de chuyen sang Komorebi.

## 4. Ket noi Claude - chon dung loai key

```powershell
wj auth
```

| Ban dang co | Chon |
| --- | --- |
| Token `sk-ant-oat...` do sep/admin cap | OAuth / session token |
| API key cua Anthropic Console | API key |
| Domain vi du `https://ai.adnx.vn` + key cua domain do | Gateway / custom domain |
| Co the dang nhap tai khoan Claude tren trinh duyet | Login |

Token duoc nhap an, khong in ra terminal. Neu may chua co Claude Code, wizard
hoi co cai bang installer chinh thuc hay khong.

### OAuth / session token

```powershell
wj auth -auth oauth -profile work -install-claude
```

Dan nguyen token `sk-ant-oat...` vao prompt.
Khong can dang nhap hay chay `claude setup-token` neu da duoc cap token hop le.
Tool se ghi `CLAUDE_CODE_OAUTH_TOKEN`.

`-auth session-key` cung co nghia nhu `-auth oauth` trong tool nay.
Cookie web `sessionKey` / `sk-ant-sid...` KHONG duoc ho tro.
Tool khong gui request de thu OAuth; Claude se kiem tra khi ban gui prompt dau tien.

### API key truc tiep

```powershell
wj auth -auth api-key -profile api -install-claude
```

Nhap key Anthropic Console. Day la API usage/billing cua key do, khong phai token
goi Pro/Max. Tool ghi `ANTHROPIC_API_KEY` va dung endpoint Anthropic chinh thuc.

### Custom domain / gateway ADNX

```powershell
wj auth -auth gateway -only claude -base-url https://ai.adnx.vn -profile adnx
```

Nhap key **do ADNX cap**. Khong dan token `sk-ant-oat` vao day.
Tool lay danh sach model, gan alias opus/sonnet/haiku/fable phu hop.

Neu gateway yeu cau header `x-api-key`:

```powershell
wj auth -auth gateway -only claude -base-url https://example.com -auth-header x-api-key
```

Neu gateway khong co API danh sach model:

```powershell
wj auth -auth gateway -only claude -base-url https://example.com -skip-check -models cc/claude-opus,cc/claude-sonnet,cc/claude-haiku
```

Thay cac model vi du bang ID that cua gateway. Neu model khong co quyen dung,
request van co the loi du da lay duoc danh sach model.

### Login binh thuong

```powershell
wj auth -auth login -install-claude
```

Tool don cau hinh key/gateway cu va mo `claude auth login --claudeai`.
Neu chi muon chuan bi cau hinh, chua dang nhap:

```powershell
wj auth -auth login -no-login
```

## 5. Doi giua ADNX va OAuth

Sau khi luu hai profile `adnx` va `work` nhu vi du tren:

```powershell
wj -list-profiles
wj -use-profile adnx -run
```

Lan sau doi sang OAuth:

```powershell
wj -use-profile work -run
```

Profile duoc luu trong `%USERPROFILE%\.wj\profiles`.
Khi doi, tool cap nhat settings Claude cua user, xoa key/base URL va model mapping
cu (vi du `cc/...`), giu hooks Orca, plugin, statusLine va cac tuy chon khac.
Tool sao luu settings truoc khi ghi.

Day la **chon mot ket noi dang dung**, khong phai tu dong chuyen du phong khi het
quota va khong tach rieng hai tai khoan chay dong thoi. Hay mo phien Claude moi
sau khi doi. Trong Claude go `/status` de xem nguon xac thuc va model.

`-run` mo Claude voi cac bien auth/model cu trong terminal da duoc loai khoi
process con. No khong xoa bien moi truong User/Machine va khong sua project hay
managed settings. Neu cong ty quan ly endpoint, chinh sach do van co hieu luc.

## 6. Loi 401 "API key required for remote API access"

Loi thuong gap: dat token `oat` vao `ANTHROPIC_AUTH_TOKEN` nhung van de
`ANTHROPIC_BASE_URL=https://ai.adnx.vn`. Request se toi gateway voi sai loai key.

De dung OAuth truc tiep:

```powershell
wj auth -auth oauth -profile work -run
```

Nhap lai token OAuth khi duoc hoi. Tool tu don URL ADNX va model `cc/...` cu.

Neu `/status` van hien endpoint cu, kiem tra settings cua project, bien moi truong
va managed settings; chung khong nam trong file user ma tool vua cap nhat.

## 6b. Do token da dung qua mitmproxy

mitmproxy dung giua Claude/Codex va gateway (vd `ai.adnx.vn`), ghi lai moi luot
goi model: thoi gian, app, model, so token vao/ra/cache. **Khong** ghi prompt,
cau tra loi hay API key. Log nam tai `~/.wj/proxy/usage.jsonl`.

**Cach 1 - mot lenh (de nhat).** Khong can cai tay gi:

```powershell
wj proxy run claude           # thieu mitmproxy thi hoi cai (Enter = co), tu bat proxy,
wj proxy run codex            # mo app; thoat app la proxy tu tat
wj proxy run -yes claude      # cho script: tu cai khong hoi
```

**Cach 2 - tung buoc** (proxy chay lau, nhieu app/cua so dung chung):

```powershell
wj proxy setup                # cai mitmproxy + tao chung chi, mot lan
wj proxy start                # cua so 1: bat proxy, de mo
wj proxy run claude           # cua so 2: tu dung proxy dang chay
```

Xem ket qua:

```powershell
wj proxy run codex exec "..." # tham so sau ten app duoc chuyen nguyen cho app
wj proxy usage                # ca may, moi app, 24 gio qua
wj proxy usage -by project    # theo tung project (-by project-app, -by model)
wj proxy usage -project remora -app codex   # loc 1 project / 1 app
wj proxy usage -since 168h    # 7 ngay; -all = toan bo
wj proxy status               # kiem tra mitmproxy, chung chi, cong, log
```

Mot log chung cho ca may va moi app (Claude, Codex, OpenCode dung chung 1 proxy
van tach duoc). Moi luot goi gan ten project: `proxy run` tu lay ten thu muc git
dang dung; dat tay bang `proxy run -project ten-du-an claude`.

**Cach 3 - bat cho ca project (Claude Code).** Khong can go `proxy run` nua:

```powershell
cd D:\du-an\remora
wj proxy project on      # ghi .claude\settings.local.json (git bo qua)
wj proxy start           # cua so rieng, de mo
claude                              # hoac claude --continue / --resume: deu duoc dem
wj proxy project list    # cac project dang bat
wj proxy project off     # tat (chi xoa phan tool da them)
```

Chi anh huong project do. Khi proxy tat, Claude trong project nay khong ket noi
duoc: bat lai `proxy start` hoac `proxy project off`. Codex chua ho tro cach nay
(Codex khong cho project tu doi proxy/provider); dung `proxy run codex`.

Ket qua mau:

```text
     APP                      MODEL  LUOT  LOI  VAO   RA  DOC CACHE  GHI CACHE    TONG
  claude  claude-haiku-4-5-20251001     1    0   10  225          0     28,417  28,652
```

- Chi app mo bang `proxy run` moi di qua proxy. Tool chi dat `HTTPS_PROXY` va
  chung chi CA (`NODE_EXTRA_CA_CERTS`, `CODEX_CA_CERTIFICATE`) cho **tien trinh do**;
  khong sua settings Claude/Codex, khong sua bien moi truong cua may.
- Lan dau `proxy start` mitmproxy tu tao CA trong `~/.mitmproxy`.
- `proxy start` chay ca do token lan web mitmweb (xem tung request tai http://127.0.0.1:8081); `-no-web` chi do token, `-web-port N` doi cong web.
- Doi cong: `proxy start -port 9000` va `proxy run -port 9000 claude`.
- Dung mitmproxy rieng cua ban (may khac/cong khac): bat no kem addon
  `~/.wj/proxy/usage_meter.py` (tao ra sau lan `proxy start` dau tien),
  roi chay `proxy run -proxy http://IP:PORT -ca duong/dan/mitmproxy-ca-cert.pem claude`.
- So token lay tu truong `usage` gateway tra ve. Gia that do gateway quyet dinh.

## 7. Xem truoc va go cai dat

```powershell
wj tools install cli -groups 1,4 -dry-run
wj auth -auth oauth -dry-run
wj tools uninstall cli
wj tools uninstall wm
```

Dry-run khong ghi file, goi mang, cai/go package hoac mo login.
Lenh go co prompt xac nhan; voi CLI, co the chi xoa khoi shell ma giu ung dung.
Neu chon go package, ung dung da co tu truoc cung co the bi go.

File auth va profile chua key tren may: khong commit, gui kem ZIP hoac chia se
file backup cho nguoi khac. ZIP do `build.ps1` tao chi co binary + launcher + docs,
khong gom token/cau hinh ca nhan.

## 8. Tu build va phat hanh

Can Go theo `go.mod`. Trong thu muc repo:

```powershell
go test ./...
go vet ./...
.\build.ps1
```

Lay `dist\wj-windows-amd64.zip` gui sang may moi.
Dung `.\build.ps1 -All` de build them Windows ARM64, macOS va Linux.

CI kiem tra Go va cu phap script; khong tu cai/go ung dung tren may test.
Workflow Release tao binary, ZIP va checksum khi push tag `v*` da duoc duyet.
Push code len nhanh Git chua tu thay the ban release ma installer dang tai.
