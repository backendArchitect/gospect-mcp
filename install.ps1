# gospect-mcp installer for Windows (PowerShell).
#
#   irm https://raw.githubusercontent.com/backendArchitect/gospect-mcp/main/install.ps1 | iex
#
# Installs to $env:LOCALAPPDATA\gospect-mcp and adds it to your user PATH. Override the version with
# $env:GOSPECT_VERSION (default: latest).
$ErrorActionPreference = "Stop"

$repo = "backendArchitect/gospect-mcp"
$bin = "gospect-mcp.exe"

$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
if ($arch -ne "amd64") { throw "gospect-mcp: only amd64 Windows binaries are published." }

$tag = $env:GOSPECT_VERSION
if (-not $tag) {
  $tag = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
if (-not $tag) { throw "gospect-mcp: could not determine the latest release." }

$asset = "gospect-mcp_${tag}_windows_${arch}.tar.gz"
$url = "https://github.com/$repo/releases/download/$tag/$asset"

$dir = Join-Path $env:LOCALAPPDATA "gospect-mcp"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$tmp = Join-Path $env:TEMP $asset

Write-Host "gospect-mcp: downloading $tag for windows/$arch ..."
Invoke-WebRequest -Uri $url -OutFile $tmp
# tar ships with Windows 10 1803+.
tar -xzf $tmp -C $dir
Remove-Item $tmp -Force

# Add the install dir to the user PATH if it isn't already there.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
  Write-Host "gospect-mcp: added $dir to your user PATH (restart your terminal to pick it up)"
}

Write-Host "gospect-mcp: installed to $dir\$bin"
& "$dir\$bin" version
