# ======================================================================
#  CAI CONG CU DONG LENH CHO WINDOWS (PowerShell, CMD, Windows Terminal, terminal trong IDE)
#   - Chon nhom: terminal dep, tim kiem/xem file, lap trinh backend, coding agent
#   - Cai bang winget, bo qua phan mem da co, chay lai bao nhieu lan cung duoc
#   - Ghi cau hinh vao 1 khoi co danh dau trong profile, khong dung phan ban tu viet
#  Go: Uninstall_DongLenh.bat
# ======================================================================
$ProgressPreference = 'SilentlyContinue'
$utf8 = New-Object Text.UTF8Encoding $false
$MARK_A = '# >>> wisjoy-cli >>>'
$MARK_B = '# <<< wisjoy-cli <<<'
$DOCS = [Environment]::GetFolderPath('MyDocuments')
# profile.ps1 = nap cho MOI cua so PowerShell: terminal thuong, Windows Terminal, terminal trong IDE (VS Code, Antigravity)
$PROFILES = @(
    (Join-Path $DOCS 'WindowsPowerShell\profile.ps1'),   # Windows PowerShell 5 (co san trong Windows)
    (Join-Path $DOCS 'PowerShell\profile.ps1')           # PowerShell 7 (pwsh)
)
# Ban cu ghi vao file chi nap cho cua so console -> terminal trong IDE khong nap
$OLD_PROFILES = @(
    (Join-Path $DOCS 'WindowsPowerShell\Microsoft.PowerShell_profile.ps1'),
    (Join-Path $DOCS 'PowerShell\Microsoft.PowerShell_profile.ps1')
)
$CLINK_DIR = Join-Path $env:LOCALAPPDATA 'clink'
$CLINK_FILES = @('wisjoy-starship.lua', 'wisjoy-aliases.lua')

# Danh sach phan mem: Nhom, Ten hien thi, ma winget, lenh de kiem tra da co chua
$PKGS = @(
    @{ G = 1; Name = 'Windows Terminal';        Id = 'Microsoft.WindowsTerminal';     Cmd = 'wt' },
    @{ G = 1; Name = 'Starship (prompt dep)';   Id = 'Starship.Starship';             Cmd = 'starship' },
    @{ G = 1; Name = 'Font JetBrainsMono Nerd'; Id = 'DEVCOM.JetBrainsMonoNerdFont';  Cmd = $null },
    @{ G = 1; Name = 'Clink (CMD xin hon)';     Id = 'chrisant996.Clink';             Cmd = $null },
    @{ G = 1; Name = 'fzf (tim mo)';            Id = 'junegunn.fzf';                  Cmd = 'fzf' },
    @{ G = 1; Name = 'zoxide (cd thong minh)';  Id = 'ajeetdsouza.zoxide';            Cmd = 'zoxide' },
    @{ G = 2; Name = 'ripgrep (rg)';            Id = 'BurntSushi.ripgrep.MSVC';       Cmd = 'rg' },
    @{ G = 2; Name = 'fd';                      Id = 'sharkdp.fd';                    Cmd = 'fd' },
    @{ G = 2; Name = 'bat';                     Id = 'sharkdp.bat';                   Cmd = 'bat' },
    @{ G = 2; Name = 'eza (ls dep)';            Id = 'eza-community.eza';             Cmd = 'eza' },
    @{ G = 2; Name = 'jq';                      Id = 'jqlang.jq';                     Cmd = 'jq' },
    @{ G = 2; Name = 'lazygit';                 Id = 'JesseDuffield.lazygit';         Cmd = 'lazygit' },
    @{ G = 2; Name = 'delta (git diff dep)';    Id = 'dandavison.delta';              Cmd = 'delta' },
    @{ G = 2; Name = 'btop';                    Id = 'aristocratos.btop4win';         Cmd = 'btop' },
    @{ G = 2; Name = 'GitHub CLI (gh)';         Id = 'GitHub.cli';                    Cmd = 'gh' },
    @{ G = 3; Name = 'Git';                     Id = 'Git.Git';                       Cmd = 'git' },
    @{ G = 3; Name = 'fnm (Node.js)';           Id = 'Schniz.fnm';                    Cmd = 'fnm' },
    @{ G = 3; Name = 'uv (Python)';             Id = 'astral-sh.uv';                  Cmd = 'uv' },
    @{ G = 3; Name = 'Neovim';                  Id = 'Neovim.Neovim';                 Cmd = 'nvim' },
    @{ G = 3; Name = 'Antigravity IDE';         Id = 'Google.AntigravityIDE';         Cmd = 'antigravity-ide' },
    @{ G = 3; Name = 'Docker Desktop';          Id = 'Docker.DockerDesktop';          Cmd = 'docker'; Ask = $true },
    @{ G = 4; Name = 'Git (Claude Code can)';   Id = 'Git.Git';                       Cmd = 'git' },
    @{ G = 4; Name = 'Claude Code';             Id = 'NATIVE-CLAUDE';                 Cmd = 'claude' },
    @{ G = 4; Name = 'Codex CLI';               Id = 'OpenAI.Codex';                  Cmd = 'codex' },
    @{ G = 4; Name = 'Antigravity CLI (agy)';   Id = 'Google.AntigravityCLI';         Cmd = 'agy' }
)
$GROUPS = [ordered]@{
    '1' = 'Terminal dep + tien: Windows Terminal (mo Windows PowerShell co san), Starship, Nerd Font, Clink cho CMD, fzf, zoxide'
    '2' = 'Tim kiem / xem file: ripgrep, fd, bat, eza, jq, lazygit, delta, btop, gh'
    '3' = 'Lap trinh backend: Git, Node (fnm), Python (uv), Neovim, Antigravity IDE, Docker (hoi rieng)'
    '4' = 'Coding agent: Claude Code, Codex CLI, Antigravity CLI (agy)'
}

function Say($msg, $color = 'Cyan') { Write-Host "`n>> $msg" -ForegroundColor $color }
function Refresh-Path { $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User') + ';' + (Join-Path $env:USERPROFILE '.local\bin') }
function Has($c) { [bool](Get-Command $c -CommandType Application -ErrorAction SilentlyContinue) }

# winget: tim ca khi PATH thieu (Windows ban Trung / may moi chua dang ky App Installer)
function Find-Winget {
    if ($script:WG) { return $script:WG }
    $c = Get-Command winget.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source
    $alias = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\winget.exe'
    if (-not $c -and (Test-Path $alias)) { $c = $alias }
    if (-not $c) {
        try { Add-AppxPackage -RegisterByFamilyName -MainPackage Microsoft.DesktopAppInstaller_8wekyb3d8bbwe -ErrorAction Stop } catch {}
        Start-Sleep 2
        if (Test-Path $alias) { $c = $alias }
    }
    if (-not $c) {
        $c = Get-ChildItem (Join-Path $env:ProgramFiles 'WindowsApps') -Filter winget.exe -Recurse -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -like '*DesktopAppInstaller*' } | Sort-Object FullName -Descending | Select-Object -First 1 -ExpandProperty FullName
    }
    $script:WG = $c
    return $c
}
function winget {
    $w = Find-Winget
    if ($w) { & $w @args; return }
    $global:LASTEXITCODE = 1
    if (-not $script:WGWarned) {
        $script:WGWarned = $true
        Say 'Khong tim thay winget. Mo Microsoft Store, tim "App Installer" -> Cai dat/Cap nhat, roi chay lai file nay.' 'Red'
    }
}

# Ghi / xoa khoi cau hinh co danh dau (chay lai bao nhieu lan cung chi co 1 khoi)
function Remove-Block($text) {
    $pat = '(?s)(\r?\n)*' + [regex]::Escape($MARK_A) + '.*?' + [regex]::Escape($MARK_B) + '(\r?\n)?'
    return [regex]::Replace($text, $pat, "`r`n")
}
function Set-Block($file, $content) {
    New-Item -ItemType Directory -Force -Path (Split-Path $file) | Out-Null
    $old = ''
    if (Test-Path $file) { $old = [IO.File]::ReadAllText($file); Copy-Item $file "$file.bak-wisjoy" -Force }
    $rest = (Remove-Block $old).Trim()
    $new = $MARK_A + "`r`n" + $content.Trim() + "`r`n" + $MARK_B + "`r`n"
    if ($rest) { $new = $rest + "`r`n`r`n" + $new }
    [IO.File]::WriteAllText($file, $new, $utf8)
}
function Clear-Block($file) {
    if (-not (Test-Path $file)) { return $false }
    $old = [IO.File]::ReadAllText($file)
    if ($old -notmatch [regex]::Escape($MARK_A)) { return $false }
    $rest = (Remove-Block $old).Trim()
    if ($rest) { [IO.File]::WriteAllText($file, $rest + "`r`n", $utf8) } else { Remove-Item $file -Force }
    return $true
}
function Find-Clink {
    $c = Get-Command clink -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source
    if ($c) { return $c }
    foreach ($d in @(${env:ProgramFiles(x86)}, $env:ProgramFiles, $env:LOCALAPPDATA)) {
        if (-not $d) { continue }
        $p = Join-Path $d 'clink\clink.bat'
        if (Test-Path $p) { return $p }
    }
    return $null
}

$PROFILE_TEXT = @'
# Tao boi Install_DongLenh.bat. Chay lai file do se ghi de khoi nay.
# Muon them cau hinh rieng thi viet BEN NGOAI khoi (truoc dong >>> hoac sau dong <<<).
function _has($c) { [bool](Get-Command $c -CommandType Application -ErrorAction SilentlyContinue) }
$env:Path += ';' + (Join-Path $env:USERPROFILE '.local\bin')

# Goi y lenh theo lich su (mui ten phai de nhan), Tab hien menu, mui ten len/xuong tim theo chu da go
if (Get-Module -ListAvailable PSReadLine) {
    Import-Module PSReadLine -ErrorAction SilentlyContinue
    if ((Get-Module PSReadLine).Version -ge [version]'2.2.0' -and -not [Console]::IsOutputRedirected) {
        Set-PSReadLineOption -PredictionSource History -PredictionViewStyle InlineView
    }
    Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete
    Set-PSReadLineKeyHandler -Key UpArrow -Function HistorySearchBackward
    Set-PSReadLineKeyHandler -Key DownArrow -Function HistorySearchForward
}

if (_has fnm)      { fnm env --use-on-cd --shell powershell | Out-String | Invoke-Expression }
if (_has zoxide)   { Invoke-Expression (& { (zoxide init powershell | Out-String) }) }
if (_has starship) { Invoke-Expression (& starship init powershell) }
if ((_has fzf) -and (Get-Module -ListAvailable PSFzf)) {
    Import-Module PSFzf
    Set-PsFzfOption -PSReadlineChordProvider 'Ctrl+t' -PSReadlineChordReverseHistory 'Ctrl+r'
}

# ---- Xem file ----
if (_has eza) {
    Remove-Item Alias:ls -Force -ErrorAction SilentlyContinue
    function ls { eza --icons --group-directories-first @args }
    function ll { eza -la --icons --git --group-directories-first @args }
    function lt { eza --tree --level=2 --icons --git-ignore @args }
} else {
    function ll { Get-ChildItem -Force @args }
}
function .. { Set-Location .. }
function ... { Set-Location ..\.. }
function mkcd($p) { New-Item -ItemType Directory -Force -Path $p | Out-Null; Set-Location $p }
function touch($p) { if (Test-Path $p) { (Get-Item $p).LastWriteTime = Get-Date } else { New-Item -ItemType File -Path $p | Out-Null } }
function which($c) { Get-Command $c -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source }
function reload { . $PROFILE.CurrentUserAllHosts }

# ---- Git ----
function g { git @args }
function gs { git status -sb @args }
function gd { git diff @args }
function glog { git log --oneline --graph --decorate -20 @args }
function lg { lazygit @args }

# ---- Coding agent ----
function cl { claude @args }
function clr { claude --resume @args }
function cx { codex @args }

function tips {
    $t = @(
        '',
        '  XEM FILE       ls / ll / lt      danh sach, chi tiet + git, dang cay',
        '                 bat file          xem file co to mau',
        '  DI CHUYEN      z ten             nhay toi thu muc hay vao (vd: z proj)',
        '                 zi                chon thu muc bang danh sach',
        '                 .. / ... / mkcd   len 1 cap / 2 cap / tao va vao thu muc',
        '  TIM KIEM       rg chu            tim chu trong toan bo code',
        '                 fd ten            tim file theo ten',
        '                 Ctrl+R / Ctrl+T   tim lenh cu / tim file (fzf)',
        '                 mui ten phai      nhan goi y lenh mo mo',
        '  GIT            g / gs / gd       git / status gon / diff',
        '                 glog / lg         lich su dang cay / lazygit',
        '  AGENT          cl / clr          claude / mo lai phien cu',
        '                 cx / agy          codex / antigravity',
        '  KHAC           btop / jq / which / touch / reload',
        ''
    )
    $t | ForEach-Object { Write-Host $_ }
}
'@

$CLINK_STARSHIP = @'
-- Tao boi Install_DongLenh.bat: prompt Starship cho CMD
if os.getenv("WISJOY_NO_STARSHIP") == nil then
  local f = io.popen("starship init cmd")
  if f then
    local s = f:read("*a")
    f:close()
    if s and #s > 0 then load(s)() end
  end
end
'@

$CLINK_ALIASES = @'
-- Tao boi Install_DongLenh.bat: lenh tat cho CMD, giong PowerShell
local a = {
  ls   = "eza --icons --group-directories-first $*",
  ll   = "eza -la --icons --git --group-directories-first $*",
  lt   = "eza --tree --level=2 --icons --git-ignore $*",
  [".."]  = "cd ..",
  ["..."] = "cd ..\\..",
  g    = "git $*",
  gs   = "git status -sb $*",
  gd   = "git diff $*",
  glog = "git log --oneline --graph --decorate -20 $*",
  lg   = "lazygit $*",
  cl   = "claude $*",
  clr  = "claude --resume $*",
  cx   = "codex $*",
  which = "where $*",
}
if os.setalias then
  for k, v in pairs(a) do os.setalias(k, v) end
end
'@

# ======================================================================
#  CHON NHOM
# ======================================================================
Write-Host ''
Write-Host '  CAI CONG CU DONG LENH (Windows)' -ForegroundColor Cyan
foreach ($k in $GROUPS.Keys) { Write-Host ("   {0}. {1}" -f $k, $GROUPS[$k]) }
$pick = $env:WJ_GROUPS
if (-not $pick) { $pick = Read-Host "`n  Chon nhom (vd: 1 2 4). Enter = tat ca, 0 = huy" }
if ($pick -match '^\s*0\s*$') { Write-Host '  Da huy, khong cai gi.'; exit 0 }
$sel = @($pick -split '[^1-4]+' | Where-Object { $_ })
if ($sel.Count -eq 0) { $sel = @('1', '2', '3', '4') }
Write-Host ("  -> Cai nhom: {0}" -f ($sel -join ', ')) -ForegroundColor Green

$results = New-Object System.Collections.ArrayList
function Note($name, $status) { [void]$results.Add([pscustomobject]@{ 'Phan mem' = $name; 'Ket qua' = $status }) }

function Install-Pkg($p) {
    if ($p.Cmd -and (Has $p.Cmd)) { Note $p.Name 'da co'; return }
    if ($p.Ask) {
        $a = Read-Host ("   Cai {0}? Nang (~1GB) va can WSL2. (y/N)" -f $p.Name)
        if ($a -notmatch '^[yY]') { Note $p.Name 'bo qua'; return }
    }
    Write-Host ("   Cai {0} ..." -f $p.Name) -NoNewline
    if ($p.Id -eq 'NATIVE-CLAUDE') {
        # Ban cai chinh thuc cua Anthropic, tu cap nhat
        $code = 0
        try { powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://claude.ai/install.ps1 | iex" | Out-Null; $code = $LASTEXITCODE } catch { $code = 1 }
    } else {
        winget install --id $p.Id -e --source winget --silent --accept-package-agreements --accept-source-agreements | Out-Null
        $code = $LASTEXITCODE
    }
    Refresh-Path
    if (($p.Cmd -and (Has $p.Cmd)) -or $code -eq 0 -or $code -eq -1978335189) {
        Write-Host ' OK' -ForegroundColor Green; Note $p.Name 'OK'
    } else {
        Write-Host (" LOI ({0})" -f $code) -ForegroundColor Red; Note $p.Name ("loi {0}" -f $code)
    }
}

# ======================================================================
#  CAI PHAN MEM
# ======================================================================
$done = @{}
foreach ($g in $sel) {
    Say ("Nhom {0}: {1}" -f $g, ($GROUPS[$g] -split ':')[0])
    foreach ($p in ($PKGS | Where-Object { "$($_.G)" -eq $g })) {
        if ($done[$p.Id]) { continue }
        $done[$p.Id] = $true
        Install-Pkg $p
    }
}
Refresh-Path

# ======================================================================
#  CAU HINH
# ======================================================================
Say 'Cau hinh'

# 1. Cho phep PowerShell chay file profile (mac dinh Windows chan)
try { Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned -Force -ErrorAction Stop; Write-Host '   ExecutionPolicy: RemoteSigned (chi tai khoan nay)' }
catch { Write-Host '   Khong doi duoc ExecutionPolicy (may cong ty khoa?) -> profile co the khong tu chay.' -ForegroundColor Yellow }

# 2. Profile PowerShell 5 + 7
foreach ($f in $OLD_PROFILES) { [void](Clear-Block $f) }   # chuyen tu cho cu, giu phan ban tu viet
foreach ($f in $PROFILES) { Set-Block $f $PROFILE_TEXT; Write-Host "   Profile: $f" }

# 2b. Starship: cho quet thu muc lau hon (thu muc nguoi dung nhieu file -> canh bao scan_timeout)
$ssCfg = Join-Path $env:USERPROFILE '.config\starship.toml'
if (-not (Test-Path $ssCfg)) {
    New-Item -ItemType Directory -Force -Path (Split-Path $ssCfg) | Out-Null
    [IO.File]::WriteAllText($ssCfg, "# Tao boi Install_DongLenh.bat`r`nscan_timeout = 200`r`ncommand_timeout = 1000`r`n", $utf8)
    Write-Host '   Starship: ~/.config/starship.toml (het canh bao scan_timeout)'
}

# 3. PSFzf (Ctrl+R / Ctrl+T dung fzf) cho Windows PowerShell
if (Has fzf) {
    powershell.exe -NoProfile -Command "if (-not (Get-Module -ListAvailable PSFzf)) { if (-not (Get-PackageProvider -ListAvailable NuGet -ErrorAction SilentlyContinue)) { Install-PackageProvider NuGet -MinimumVersion 2.8.5.201 -Scope CurrentUser -Force | Out-Null }; Install-Module PSFzf -Scope CurrentUser -Force -ErrorAction Stop }" 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { Write-Host '   PSFzf: OK (Ctrl+R, Ctrl+T)' } else { Write-Host '   PSFzf: khong cai duoc (bo qua)' -ForegroundColor Yellow }
}

# 4. CMD: Clink tu chay + Starship + alias giong PowerShell
$clink = Find-Clink
if ($clink) {
    New-Item -ItemType Directory -Force -Path $CLINK_DIR | Out-Null
    [IO.File]::WriteAllText((Join-Path $CLINK_DIR 'wisjoy-starship.lua'), $CLINK_STARSHIP, $utf8)
    [IO.File]::WriteAllText((Join-Path $CLINK_DIR 'wisjoy-aliases.lua'), $CLINK_ALIASES, $utf8)
    & $clink autorun install 2>&1 | Out-Null
    Write-Host '   CMD: Clink + Starship + alias (mo cua so CMD moi de thay)'
}

# 5. Windows Terminal: font Nerd Font + mo Windows PowerShell (co san) mac dinh
$wtFiles = @(
    (Join-Path $env:LOCALAPPDATA 'Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json'),
    (Join-Path $env:LOCALAPPDATA 'Packages\Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe\LocalState\settings.json'),
    (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows Terminal\settings.json')
)
$wtDone = $false
foreach ($p in $wtFiles) {
    if (-not (Test-Path $p)) { continue }
    $raw = [IO.File]::ReadAllText($p)
    $clean = ($raw -split "`r?`n" | Where-Object { $_ -notmatch '^\s*//' }) -join "`n"
    try { $j = $clean | ConvertFrom-Json -ErrorAction Stop } catch { Write-Host "   Windows Terminal: khong doc duoc file cai dat -> tu chon font trong Settings" -ForegroundColor Yellow; continue }
    if ($j.profiles -isnot [pscustomobject]) { continue }
    Copy-Item $p "$p.bak-wisjoy" -Force
    if (-not $j.profiles.PSObject.Properties['defaults']) { $j.profiles | Add-Member -NotePropertyName defaults -NotePropertyValue ([pscustomobject]@{}) }
    $font = [pscustomobject]@{ face = 'JetBrainsMono Nerd Font' }
    if ($j.profiles.defaults.PSObject.Properties['font']) { $j.profiles.defaults.font = $font } else { $j.profiles.defaults | Add-Member -NotePropertyName font -NotePropertyValue $font }
    # Mot bo PowerShell duy nhat: Windows PowerShell 5.1 co san, khong go duoc
    $winPsGuid = '{61c54bbd-c2c6-5271-96e7-009a87ff44bf}'
    if (@($j.profiles.list | Where-Object { $_.guid -eq $winPsGuid }).Count -gt 0) { $j.defaultProfile = $winPsGuid }
    [IO.File]::WriteAllText($p, ($j | ConvertTo-Json -Depth 50), $utf8)
    $wtDone = $true
}
if ($wtDone) { Write-Host '   Windows Terminal: font JetBrainsMono Nerd Font, mo Windows PowerShell mac dinh' }
elseif (Has wt) { Write-Host '   Windows Terminal: chua co file cai dat -> mo Windows Terminal 1 lan roi chay lai file nay' -ForegroundColor Yellow }

# 6. Git
if (Has git) {
    git config --global init.defaultBranch main
    if (Has delta) {
        git config --global core.pager delta
        git config --global interactive.diffFilter 'delta --color-only'
        git config --global delta.navigate true
        git config --global merge.conflictstyle zdiff3
    }
    if (-not (git config --global user.name)) { $n = Read-Host '   Ten hien thi cho git (Enter = bo qua)'; if ($n) { git config --global user.name $n } }
    if (-not (git config --global user.email)) { $e = Read-Host '   Email cho git (Enter = bo qua)'; if ($e) { git config --global user.email $e } }
    Write-Host '   Git: nhanh mac dinh main' $(if (Has delta) { '+ diff bang delta' })
}

# 7. Node LTS qua fnm
if (Has fnm) {
    $hasNode = (fnm list 2>$null | Out-String) -match 'v\d'
    if (-not $hasNode) { Write-Host '   Node.js: cai ban LTS...'; fnm install --lts 2>&1 | Out-Null }
    fnm default lts-latest 2>&1 | Out-Null
    Write-Host '   Node.js: da co ban LTS (fnm)'
}

# ======================================================================
#  KET QUA
# ======================================================================
Say 'Ket qua' 'Green'
$results | Format-Table -AutoSize | Out-String | Write-Host
Write-Host '  Dong cua so nay, mo Windows Terminal moi, go  tips  de xem cac lenh tat.' -ForegroundColor Green
Write-Host '  Lan dau chay: claude / codex / agy se mo trinh duyet de dang nhap.'
$bad = @($results | Where-Object { $_.'Ket qua' -like 'loi*' })
if ($bad.Count) {
    Write-Host '  Cai loi phan nao: chay lai wj tools install cli.' -ForegroundColor Yellow
    exit 1
}
exit 0
