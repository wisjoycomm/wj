param(
    [string]$Version = "latest",
    [switch]$RunSetup,
    [string]$InstallDir = "",
    [switch]$NoPath
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$repo = "wisjoycomm/wj"
$dir = if ($InstallDir) { [IO.Path]::GetFullPath($InstallDir) } else { Join-Path $env:USERPROFILE ".local\bin" }
$arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$asset = switch ($arch) {
    "AMD64" { "wj.exe" }
    "ARM64" { "wj-windows-arm64.exe" }
    default { throw "Unsupported Windows architecture: $arch. Use 64-bit Windows." }
}
$releaseUrl = if ($Version -eq "latest") {
    "https://api.github.com/repos/$repo/releases/latest"
} else {
    "https://api.github.com/repos/$repo/releases/tags/$([uri]::EscapeDataString($Version))"
}
$release = Invoke-RestMethod -Uri $releaseUrl -Headers @{ "User-Agent" = "wj-installer" }
$binary = @($release.assets | Where-Object name -eq $asset)
$checksumAsset = @($release.assets | Where-Object name -eq "checksums.txt")
if ($binary.Count -ne 1) { throw "Release $($release.tag_name) has no $asset build." }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$dest = Join-Path $dir "wj.exe"
$download = Join-Path $dir (".wj-" + [guid]::NewGuid().ToString("N") + ".tmp")
function Get-InstallerHash([string]$Path) {
    $stream = [IO.File]::OpenRead($Path)
    $hasher = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($hasher.ComputeHash($stream))).Replace('-', '') }
    finally { $hasher.Dispose(); $stream.Dispose() }
}
try {
    $expected = ""
    if ($binary[0].digest -match '^sha256:([a-fA-F0-9]{64})$') {
        $expected = $Matches[1]
    } elseif ($checksumAsset.Count -eq 1) {
        $checksums = (Invoke-WebRequest -UseBasicParsing -Uri $checksumAsset[0].browser_download_url).Content
        foreach ($line in ($checksums -split "`n")) {
            if ($line.Trim() -match ('^([a-fA-F0-9]{64})\s+\*?' + [regex]::Escape($asset) + '$')) { $expected = $Matches[1] }
        }
    }
    if (-not $expected) { throw "Release has no SHA-256 for $asset; installation stopped." }
    if ((Test-Path -LiteralPath $dest) -and (Get-InstallerHash $dest) -eq $expected) {
        Write-Host 'This release is already installed. Reusing the local executable.'
    } else {
        Invoke-WebRequest -UseBasicParsing -Uri $binary[0].browser_download_url -OutFile $download
        if ((Get-InstallerHash $download) -ne $expected) { throw "SHA-256 mismatch; existing installation was not replaced." }
        Move-Item -LiteralPath $download -Destination $dest -Force
    }
} finally {
    if (Test-Path -LiteralPath $download) { Remove-Item -LiteralPath $download -Force }
}
if (-not $NoPath) {
    $userPath = [string][Environment]::GetEnvironmentVariable("Path", "User")
    if (($userPath -split ';') -notcontains $dir) {
        [Environment]::SetEnvironmentVariable("Path", (($userPath.TrimEnd(';') + ";" + $dir).TrimStart(';')), "User")
        # Notify Explorer so terminals opened from Start inherit the new PATH.
        try {
            if (-not ('Wisjoy.EnvironmentNotification' -as [type])) {
                Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace Wisjoy {
    public static class EnvironmentNotification {
        [DllImport("user32.dll", CharSet = CharSet.Auto, SetLastError = true)]
        public static extern IntPtr SendMessageTimeout(IntPtr window, uint message,
            UIntPtr wParam, string lParam, uint flags, uint timeout, out UIntPtr result);
    }
}
'@
            }
            $notificationResult = [UIntPtr]::Zero
            [Wisjoy.EnvironmentNotification]::SendMessageTimeout([IntPtr]0xffff, 0x001a,
                [UIntPtr]::Zero, 'Environment', 2, 1000, [ref]$notificationResult) | Out-Null
        } catch { Write-Host 'PATH saved. Restart your terminal application if the command is not found.' }
    }
    if (($env:Path -split ';') -notcontains $dir) { $env:Path = "$dir;$env:Path" }
}
Write-Host "Installed $($release.tag_name): $dest (SHA-256 verified)"
Write-Host "Installed. Open a new terminal and run: wj   (add -RunSetup to the installer to open it right away). Use wj update only when you want a new release."
if ($RunSetup) {
    & $dest
    if ($LASTEXITCODE -ne 0) { throw "Setup exited with code $LASTEXITCODE. Run the installed tool again to retry." }
}
