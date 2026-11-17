# ======================================================================
#  CAI WSL2 + UBUNTU CHO WINDOWS
#   - Bat WSL (can quyen admin + khoi dong lai, chi lan dau)
#   - Ghi %UserProfile%\.wslconfig: gioi han RAM, tu tra RAM/o dia
#   - Cai Ubuntu, dat lam distro mac dinh (Docker Desktop tu noi vao)
#   - Chay lai bao nhieu lan cung duoc; khong dung toi phan ban tu viet
# ======================================================================
$ProgressPreference = 'SilentlyContinue'
$env:WSL_UTF8 = '1'   # wsl.exe in ra UTF-8 thay vi UTF-16
$utf8 = New-Object Text.UTF8Encoding $false
$MARK_A = '# >>> wisjoy-wsl >>>'
$MARK_B = '# <<< wisjoy-wsl <<<'
$DISTRO = if ($env:WJ_WSL_DISTRO) { $env:WJ_WSL_DISTRO } else { 'Ubuntu' }
$CFG = Join-Path $env:USERPROFILE '.wslconfig'

function Say($msg, $color = 'Cyan') { Write-Host "`n>> $msg" -ForegroundColor $color }
function Distros { @(wsl.exe --list --quiet 2>$null | ForEach-Object { "$_".Trim([char]0, ' ') } | Where-Object { $_ }) }

# 1. WSL co chua? (wsl --version chi chay duoc khi WSL ban moi da cai)
Say 'WSL'
wsl.exe --version 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host '   Chua co WSL -> cai (Windows se hoi quyen admin)...'
    try {
        $p = Start-Process wsl.exe -ArgumentList '--install', '--no-distribution' -Verb RunAs -Wait -PassThru -ErrorAction Stop
        $code = $p.ExitCode
    } catch { $code = 1 }
    if ($code -ne 0) { Write-Host "   Cai WSL loi ($code). Bat 'Virtualization' trong BIOS roi chay lai." -ForegroundColor Red; exit 1 }
    Write-Host '   Da cai WSL. KHOI DONG LAI MAY roi chay lai: wj tools install wsl' -ForegroundColor Yellow
    exit 0
}
wsl.exe --update 2>&1 | Out-Null
Write-Host ('   ' + ((wsl.exe --version 2>$null | Select-Object -First 1) -replace '\s+', ' ').Trim())

# 2. .wslconfig: khoi co danh dau, khong dung toi cau hinh ban tu viet
Say '.wslconfig'
$ramGB = [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1GB)
$memGB = [math]::Max(2, [math]::Floor($ramGB / 2))
$block = @"
$MARK_A
# Tao boi wj tools install wsl. Chay lai se ghi de khoi nay.
[wsl2]
memory=${memGB}GB
[experimental]
autoMemoryReclaim=dropCache
sparseVhd=true
$MARK_B
"@
$old = if (Test-Path $CFG) { [IO.File]::ReadAllText($CFG) } else { '' }
$pat = '(?s)' + [regex]::Escape($MARK_A) + '.*?' + [regex]::Escape($MARK_B)
if ($old -match [regex]::Escape($MARK_A)) {
    Copy-Item $CFG "$CFG.bak-wisjoy" -Force
    [IO.File]::WriteAllText($CFG, [regex]::Replace($old, $pat, $block.Trim()), $utf8)
    Write-Host "   Cap nhat: RAM toi da ${memGB}GB / ${ramGB}GB, tu tra RAM va o dia"
} elseif ($old -match '(?m)^\s*\[(wsl2|experimental)\]') {
    Write-Host "   Da co cau hinh rieng trong $CFG -> giu nguyen" -ForegroundColor Yellow
} else {
    if ($old) { Copy-Item $CFG "$CFG.bak-wisjoy" -Force }
    $new = if ($old.Trim()) { $old.TrimEnd() + "`r`n`r`n" + $block } else { $block }
    [IO.File]::WriteAllText($CFG, $new.Trim() + "`r`n", $utf8)
    Write-Host "   Ghi: RAM toi da ${memGB}GB / ${ramGB}GB, tu tra RAM va o dia"
}
if (@(Distros).Count -gt 0) { Write-Host '   Ap dung tu lan khoi dong WSL sau (wsl --shutdown hoac khoi dong lai may).' }

# 3. Ubuntu
Say $DISTRO
$fresh = $false
if ((Distros) -contains $DISTRO) {
    Write-Host '   Da co'
} else {
    Write-Host '   Cai (tai ~1GB)...'
    wsl.exe --install --distribution $DISTRO --no-launch
    if ((Distros) -notcontains $DISTRO) { Write-Host "   Cai $DISTRO loi." -ForegroundColor Red; exit 1 }
    $fresh = $true
}
wsl.exe --set-default $DISTRO | Out-Null
Write-Host "   Distro mac dinh: $DISTRO (lenh wsl mo thang vao day)"

# 4. Docker Desktop tu noi vao distro mac dinh; Docker Engine rieng trong WSL thi khong can
Say 'Docker'
# Cai cho ca may (Program Files) hoac chi tai khoan nay (LocalAppData)
$dd = @((Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\DockerDesktop\Docker Desktop.exe')) | Where-Object { Test-Path $_ }
if ($dd) {
    Write-Host "   Docker Desktop tu noi vao distro mac dinh ($DISTRO) sau khi khoi dong lai Docker Desktop."
    Write-Host "   Kiem tra: wsl -d $DISTRO -e docker ps. Chua chay thi: Settings > Resources > WSL Integration > bat $DISTRO."
} else {
    Write-Host '   Chua co Docker Desktop. Can thi: wj tools install cli -groups 3'
}

# 5. Tai khoan Linux: phai do nguoi go (mat khau), lam 1 lan
$user = (wsl.exe -d $DISTRO -e sh -c 'getent passwd 1000 | cut -d: -f1' 2>$null | Out-String).Trim()
Say 'Ket qua' 'Green'
if (-not $user) {
    Write-Host '  Con 1 buoc: tao tai khoan Linux. Mo terminal moi, go:' -ForegroundColor Yellow
    Write-Host "     wsl -d $DISTRO"
    Write-Host '  nhap ten + mat khau (mat khau dung cho sudo), roi go exit.'
} else {
    Write-Host "  Tai khoan Linux: $user"
}
Write-Host '  Code chay trong Linux/Docker: de o ~/projects trong WSL, khong de o /mnt/d (cham).'
Write-Host '  Mo thu muc WSL tu Windows: \\wsl$\' -NoNewline; Write-Host $DISTRO
exit 0
