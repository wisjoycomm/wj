# ======================================================================
#  GO CONG CU DONG LENH (Windows)
#   - Luon xoa cau hinh da them (profile, Clink, git delta)
#   - Phan mem: chi go nhom ban chon
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
    @{ G = 1; Name = 'PowerShell 7';            Id = 'Microsoft.PowerShell';          Cmd = 'pwsh' },
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
    '1' = 'Terminal dep + tien: PowerShell 7 (neu ban cu da cai), Windows Terminal, Starship, Nerd Font, Clink cho CMD, fzf, zoxide'
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
    if ((Get-Module PSReadLine).Version -ge [version]'2.2.0') {
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
#  GO CAU HINH (luon lam) + GO PHAN MEM (hoi truoc)
# ======================================================================
Write-Host ''
Write-Host '  GO CONG CU DONG LENH (Windows)' -ForegroundColor Cyan

Say 'Xoa cau hinh da them (giu nguyen phan ban tu viet)'
foreach ($f in ($PROFILES + $OLD_PROFILES)) { if (Clear-Block $f) { Write-Host "   Profile: da xoa khoi wisjoy trong $f" } }
foreach ($n in $CLINK_FILES) { $p = Join-Path $CLINK_DIR $n; if (Test-Path $p) { Remove-Item $p -Force; Write-Host "   Clink: xoa $n" } }
if (Get-Command git -ErrorAction SilentlyContinue) {
    if ((git config --global core.pager) -eq 'delta') {
        git config --global --unset core.pager
        git config --global --unset interactive.diffFilter
        git config --global --unset delta.navigate
        Write-Host '   Git: bo delta, tro ve diff mac dinh'
    }
}

Write-Host ''
Write-Host '  Go luon phan mem? Chon nhom (vd: 2 4), Enter = KHONG go gi:'
foreach ($k in $GROUPS.Keys) { Write-Host ("   {0}. {1}" -f $k, $GROUPS[$k]) }
$pick = $env:WJ_GROUPS
if (-not $pick) { $pick = Read-Host '  Nhom' }
$sel = @($pick -split '[^1-4]+' | Where-Object { $_ })
if ($sel.Count -gt 0) {
    $keep = @('Git.Git', 'Microsoft.WindowsTerminal', 'Microsoft.PowerShell')
    $done = @{}
    foreach ($p in ($PKGS | Where-Object { $sel -contains "$($_.G)" })) {
        if ($done[$p.Id]) { continue }
        $done[$p.Id] = $true
        if ($keep -contains $p.Id) {
            $a = Read-Host ("   Go ca {0}? Nhieu phan mem khac can no. (y/N)" -f $p.Name)
            if ($a -notmatch '^[yY]') { continue }
        }
        Write-Host ("   Go {0} ..." -f $p.Name)
        if ($p.Id -eq 'NATIVE-CLAUDE') {
            Remove-Item (Join-Path $env:USERPROFILE '.local\bin\claude.exe') -Force -ErrorAction SilentlyContinue
            Remove-Item (Join-Path $env:USERPROFILE '.local\share\claude') -Recurse -Force -ErrorAction SilentlyContinue
            winget uninstall --id Anthropic.ClaudeCode -e --silent 2>$null | Out-Null
        } else {
            winget uninstall --id $p.Id -e --silent 2>$null | Out-Null
        }
    }
    $clinkLeft = Find-Clink
    if (-not $clinkLeft) { Write-Host '   Clink da go -> CMD tro ve binh thuong' }
}
Say 'XONG. Mo cua so terminal moi de thay thay doi.' 'Green'
Write-Host '  Ban sao cau hinh cu nam canh file goc, duoi .bak-wisjoy (profile, settings.json Windows Terminal).'
