# ⚡ redstone installer (windows) — one static binary on your PATH, verified first.
#
#   irm https://raw.githubusercontent.com/tomblancdev/redstone/main/scripts/install.ps1 | iex
#
# Pin a version:     $env:REDSTONE_VERSION = "v0.1.0"; irm ... | iex
# Custom directory:  $env:REDSTONE_INSTALL = "C:\Tools"; irm ... | iex
#                    (custom directories are left off PATH — your call)
# Updating IS installing: run it again, the binary is swapped (state is
# files and is never touched).
$ErrorActionPreference = "Stop"

$Repo = "tomblancdev/redstone"
$Version = $env:REDSTONE_VERSION
$CustomDest = [bool]$env:REDSTONE_INSTALL
$Dest = if ($CustomDest) { $env:REDSTONE_INSTALL } else { Join-Path $env:LOCALAPPDATA "Programs\redstone" }

if ($env:PROCESSOR_ARCHITECTURE -ne "AMD64") {
  throw "only windows-amd64 builds exist (this machine: $env:PROCESSOR_ARCHITECTURE)"
}

if (-not $Version) {
  $Version = (Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest").tag_name
}

$Bin = "redstone-windows-amd64.exe"
$Base = "https://github.com/$Repo/releases/download/$Version"
$Tmp = Join-Path $env:TEMP "redstone-install-$PID"
New-Item -ItemType Directory -Force $Tmp | Out-Null

Write-Host "fetching redstone $Version (windows/amd64)"
Invoke-WebRequest "$Base/$Bin" -OutFile (Join-Path $Tmp $Bin) -UseBasicParsing
Invoke-WebRequest "$Base/checksums.txt" -OutFile (Join-Path $Tmp "checksums.txt") -UseBasicParsing

# verify before placing — never trust a download you didn't test
$Expected = ((Select-String -Path (Join-Path $Tmp "checksums.txt") -Pattern ([regex]::Escape($Bin))).Line -split "\s+")[0]
$Actual = (Get-FileHash (Join-Path $Tmp $Bin) -Algorithm SHA256).Hash
if ($Expected.ToLower() -ne $Actual.ToLower()) { throw "checksum mismatch - refusing to install" }
Write-Host "checksum verified"

New-Item -ItemType Directory -Force $Dest | Out-Null
Move-Item -Force (Join-Path $Tmp $Bin) (Join-Path $Dest "redstone.exe")
Remove-Item -Recurse -Force $Tmp

if (-not $CustomDest) {
  # default location earns a user-PATH entry; new terminals will see it
  $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if (($UserPath -split ";") -notcontains $Dest) {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$Dest", "User")
    Write-Host "added $Dest to your user PATH - open a new terminal to use 'redstone'"
  }
}

Write-Host "placed: $(& (Join-Path $Dest "redstone.exe") version)"
