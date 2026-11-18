<#
.SYNOPSIS
  A Windowsra telepített Kinora frissítése a legújabb kiadott verzióra, hiba esetén visszaállással.

.DESCRIPTION
  Az install.ps1 által felvett ütemezett feladat futtatja naponta (SYSTEM-ként), kézzel is indítható.
  Csak kiadott verzióra (GitHub Release, v*.*.* tag) frissít; ha nincs újabb, nem csinál semmit.
  Lépések: letöltés + SHA-256 ellenőrzés → a szolgáltatás leállítása → a program és az adatbázis mentése →
  csere → indítás → állapotellenőrzés (/api/status). Ha az új verzió 90 másodpercen belül nem válaszol,
  visszaállítja a régi programot ÉS az adatbázist (az új verzió migrációja miatt), majd újraindítja.
  Napló: <InstallDir>\update.log

.EXAMPLE
  .\update.ps1            # frissít, ha van újabb kiadás
  .\update.ps1 -Force     # újratelepíti a legújabb kiadást akkor is, ha már az fut
#>
[CmdletBinding()]
param(
  [string]$InstallDir = $PSScriptRoot,
  [string]$Repo = "ScotyLucas/kinora",
  [switch]$Force
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$asset = "kinora-windows-amd64.zip"
$exe = Join-Path $InstallDir "kinora.exe"
$logFile = Join-Path $InstallDir "update.log"

function Log([string]$msg) {
  $line = "{0}  {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $msg
  Write-Host $line
  Add-Content -Path $logFile -Value $line -Encoding UTF8
}

function ParseVersion([string]$s) {
  # "kinora v1.2.3" / "v1.2.3" → [version]1.2.3; minden más (dev, edge-…) → 0.0.0, így arra mindig frissít
  if ($s -match 'v?(\d+)\.(\d+)\.(\d+)') { return [version]"$($Matches[1]).$($Matches[2]).$($Matches[3])" }
  return [version]"0.0.0"
}

# a szolgáltatás indítási argumentumaiból: --data és --addr (az install.ps1 ezeket adja meg)
function ServiceArg([string]$imagePath, [string]$name, [string]$default) {
  if ($imagePath -match ("--$name(?:=|\s+)(?:""([^""]*)""|(\S+))")) {
    if ($Matches[1]) { return $Matches[1] } else { return $Matches[2] }
  }
  return $default
}

function StopKinora {
  $s = Get-Service -Name Kinora
  if ($s.Status -ne "Stopped") {
    Stop-Service -Name Kinora -Force
    $s.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(45))
  }
  # a szolgáltatás leállta után az ffmpeg-gyerekfolyamatok is zárolhatják a fájlokat
  Get-Process -Name ffmpeg, ffprobe -ErrorAction SilentlyContinue |
    Where-Object { $_.Path -and $_.Path.StartsWith($InstallDir, [StringComparison]::OrdinalIgnoreCase) } |
    Stop-Process -Force -ErrorAction SilentlyContinue
}

function Healthy([int]$port, [int]$seconds) {
  $deadline = (Get-Date).AddSeconds($seconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/status" -TimeoutSec 4
      if ($r.product -eq "kinora") { return $true }
    } catch { }
    Start-Sleep -Seconds 2
  }
  return $false
}

# a mentett program és adatbázis visszamásolása, majd indítás
function Restore {
  StopKinora
  Get-ChildItem -Path (Join-Path $backup "*") -Exclude "db" | Copy-Item -Destination $InstallDir -Recurse -Force
  Get-ChildItem -Path $dataDir -Filter "kinora.db*" -File | Remove-Item -Force
  Get-ChildItem -Path (Join-Path $backup "db") -File | Copy-Item -Destination $dataDir -Force
  Start-Service -Name Kinora
  if (Healthy $port 60) { Log "Visszaállítva: $current" } else { Log "HIBA: a visszaállított verzió sem indul, nézd meg: $dataDir\kinora.log" }
}

$work = $null
$backupReady = $false
try {
  if (-not (Test-Path $exe)) { throw "Nincs kinora.exe itt: $InstallDir" }
  $svcKey = "HKLM:\SYSTEM\CurrentControlSet\Services\Kinora"
  if (-not (Test-Path $svcKey)) { throw "A Kinora szolgáltatás nincs telepítve (install.ps1)." }
  $imagePath = (Get-ItemProperty $svcKey).ImagePath
  $dataDir = ServiceArg $imagePath "data" "$env:ProgramData\Kinora"
  $addr = ServiceArg $imagePath "addr" ":8096"
  $port = [int]($addr -replace '^.*:', '')

  $current = (& $exe version) -join " "
  $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "kinora-updater" }
  $have = ParseVersion $current
  $want = ParseVersion $rel.tag_name
  if (-not $Force -and $want -le $have) {
    Write-Host "Naprakész: $current (legújabb kiadás: $($rel.tag_name))"
    exit 0
  }
  $za = $rel.assets | Where-Object name -eq $asset
  $sa = $rel.assets | Where-Object name -eq "$asset.sha256"
  if (-not $za -or -not $sa) { Log "A $($rel.tag_name) kiadásban nincs Windows-csomag, kihagyva."; exit 0 }

  Log "Frissítés: $current → $($rel.tag_name)"
  $work = Join-Path $env:TEMP ("kinora-update-" + [guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $work | Out-Null
  $zip = Join-Path $work $asset
  Invoke-WebRequest -Uri $za.browser_download_url -OutFile $zip -UseBasicParsing
  Invoke-WebRequest -Uri $sa.browser_download_url -OutFile "$zip.sha256" -UseBasicParsing
  $sum = ((Get-Content "$zip.sha256" -Raw).Trim() -split '\s+')[0].ToLower()
  if ($sum -ne (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()) { throw "SHA-256 eltérés, a frissítés megszakítva." }

  $stage = Join-Path $work "stage"
  Expand-Archive -Path $zip -DestinationPath $stage
  $newVer = (& (Join-Path $stage "kinora.exe") version) -join " "
  if ((ParseVersion $newVer) -ne $want) { throw "A csomagban lévő kinora.exe verziója váratlan: $newVer" }

  # mentés: a régi program és (leállított szolgáltatásnál, tehát konzisztensen) az adatbázis
  $backup = Join-Path $InstallDir "backup"
  Remove-Item -Recurse -Force $backup -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Path (Join-Path $backup "db") | Out-Null
  StopKinora
  Get-ChildItem -Path (Join-Path $InstallDir "*") -Exclude "backup", "update.log" | Copy-Item -Destination $backup -Recurse -Force
  Get-ChildItem -Path $dataDir -Filter "kinora.db*" -File -ErrorAction SilentlyContinue |
    Copy-Item -Destination (Join-Path $backup "db") -Force
  $backupReady = $true
  Log "Mentés kész: $backup"

  Copy-Item -Path (Join-Path $stage "*") -Destination $InstallDir -Recurse -Force
  Start-Service -Name Kinora
  if (Healthy $port 90) {
    Log "Sikeres frissítés: $newVer"
    exit 0
  }

  # visszaállás
  Log "Az új verzió nem válaszol a $port porton – visszaállás erre: $current"
  Restore
  exit 1
}
catch {
  Log "HIBA: $($_.Exception.Message)"
  if ($backupReady) {
    # a csere közben történt a hiba: a régi állapot visszaállítása
    try { Restore } catch { Log "HIBA a visszaállításkor: $($_.Exception.Message)" }
  } elseif ((Get-Service -Name Kinora -ErrorAction SilentlyContinue).Status -eq "Stopped") {
    # a mentés előtt állt le: ne maradjon leállva a szerver
    Start-Service -Name Kinora -ErrorAction SilentlyContinue
  }
  exit 1
}
finally {
  if ($work) { Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue }
}
