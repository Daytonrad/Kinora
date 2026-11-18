<#
.SYNOPSIS
  Kinora telepítése Windowsra (Windows Server is), Windows-szolgáltatásként, automatikus frissítéssel.

.DESCRIPTION
  Letölti a legújabb kiadást (GitHub Release, kinora-windows-amd64.zip), ellenőrzi az SHA-256 összeget,
  kicsomagolja az InstallDir-be, telepíti és elindítja a "Kinora" szolgáltatást, megnyitja a tűzfalat,
  és felvesz egy ütemezett feladatot, amely naponta lefuttatja az update.ps1-et (csak kiadott verziókra frissít).
  Újrafuttatva frissíti a telepítést; a meglévő adatokat (DataDir) nem bántja.

.EXAMPLE
  # rendszergazdai PowerShellben:
  .\install.ps1 -Media "Filmek=D:\Filmek;Sorozatok=D:\Sorozatok"

.EXAMPLE
  # a már letöltött zipből, frissítés nélkül:
  .\install.ps1 -ZipPath .\kinora-windows-amd64.zip -NoAutoUpdate
#>
[CmdletBinding()]
param(
  [string]$InstallDir = "$env:ProgramFiles\Kinora",
  [string]$DataDir = "$env:ProgramData\Kinora",
  [int]$Port = 8096,
  # első indításkor felvett médiamappák, ';'-vel elválasztva, opcionálisan Név=útvonal
  [string]$Media = "",
  # helyi zip a letöltés helyett (ilyenkor nincs SHA-256 ellenőrzés, ha nincs mellette .sha256 fájl)
  [string]$ZipPath = "",
  # a napi frissítés időpontja (helyi idő)
  [string]$UpdateTime = "04:00",
  [switch]$NoAutoUpdate,
  [string]$Repo = "ScotyLucas/kinora"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"   # az Invoke-WebRequest folyamatjelzője PS 5.1-ben nagyon lassít
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
  [Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Rendszergazdaként futtasd (jobb klikk a PowerShellen → Futtatás rendszergazdaként)." }

$asset = "kinora-windows-amd64.zip"
$work = Join-Path $env:TEMP ("kinora-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $work | Out-Null

try {
  # 1. a csomag beszerzése
  if ($ZipPath) {
    $zip = (Resolve-Path $ZipPath).Path
    $sumFile = "$zip.sha256"
  } else {
    Write-Host "Legújabb kiadás keresése: $Repo"
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "kinora-installer" }
    $za = $rel.assets | Where-Object name -eq $asset
    $sa = $rel.assets | Where-Object name -eq "$asset.sha256"
    if (-not $za -or -not $sa) { throw "A $($rel.tag_name) kiadásban nincs Windows-csomag." }
    $zip = Join-Path $work $asset
    $sumFile = "$zip.sha256"
    Write-Host "Letöltés: $($rel.tag_name)"
    Invoke-WebRequest -Uri $za.browser_download_url -OutFile $zip -UseBasicParsing
    Invoke-WebRequest -Uri $sa.browser_download_url -OutFile $sumFile -UseBasicParsing
  }
  if (Test-Path $sumFile) {
    $want = ((Get-Content $sumFile -Raw).Trim() -split '\s+')[0].ToLower()
    $got = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
    if ($want -ne $got) { throw "SHA-256 eltérés: a letöltött csomag sérült vagy nem hiteles." }
    Write-Host "SHA-256 rendben."
  } else {
    Write-Warning "Nincs .sha256 fájl a zip mellett, az ellenőrzés kimarad."
  }

  # 2. kicsomagolás; futó szolgáltatásnál előbb leállítjuk (a kinora.exe zárolva van)
  $stage = Join-Path $work "stage"
  Expand-Archive -Path $zip -DestinationPath $stage
  if (-not (Test-Path (Join-Path $stage "kinora.exe"))) { throw "A csomagban nincs kinora.exe." }
  $svc = Get-Service -Name Kinora -ErrorAction SilentlyContinue
  if ($svc -and $svc.Status -ne "Stopped") {
    Write-Host "A futó Kinora szolgáltatás leállítása…"
    Stop-Service -Name Kinora -Force
    (Get-Service Kinora).WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
  }
  New-Item -ItemType Directory -Force -Path $InstallDir, $DataDir | Out-Null
  Copy-Item -Path (Join-Path $stage "*") -Destination $InstallDir -Recurse -Force
  $exe = Join-Path $InstallDir "kinora.exe"
  Write-Host ("Telepítve: " + (& $exe version) + " → $InstallDir")

  # 3. szolgáltatás (az install újratelepít, ha már létezik, és el is indítja)
  $svcArgs = @("service", "install", "--data", $DataDir, "--addr", ":$Port")
  if ($Media) { $svcArgs += @("--initial-media", $Media) }
  & $exe @svcArgs
  if ($LASTEXITCODE -ne 0) { throw "A szolgáltatás telepítése nem sikerült (kód: $LASTEXITCODE)." }

  # 4. tűzfal: webes felület + az appok szerverkeresése (UDP 7359)
  foreach ($r in @(
      @{ Name = "Kinora (TCP $Port)"; Protocol = "TCP"; Port = $Port },
      @{ Name = "Kinora szerverkeresés (UDP 7359)"; Protocol = "UDP"; Port = 7359 })) {
    Get-NetFirewallRule -DisplayName $r.Name -ErrorAction SilentlyContinue | Remove-NetFirewallRule
    New-NetFirewallRule -DisplayName $r.Name -Direction Inbound -Action Allow -Protocol $r.Protocol `
      -LocalPort $r.Port -Profile Domain, Private | Out-Null
  }
  Write-Host "Tűzfal: $Port/TCP és 7359/UDP engedélyezve (tartomány és magánhálózat)."

  # 5. automatikus frissítés: naponta, SYSTEM-ként, csak kiadott verziókra
  $taskName = "Kinora frissítés"
  Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
  if (-not $NoAutoUpdate) {
    $script = Join-Path $InstallDir "update.ps1"
    $action = New-ScheduledTaskAction -Execute "powershell.exe" `
      -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$script`" -Repo $Repo"
    $trigger = New-ScheduledTaskTrigger -Daily -At $UpdateTime -RandomDelay (New-TimeSpan -Minutes 30)
    $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Hours 1)
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings `
      -Principal $principal -Description "Kinora frissítése a legújabb kiadásra (update.ps1)" | Out-Null
    Write-Host "Automatikus frissítés: naponta $UpdateTime körül (ütemezett feladat: '$taskName')."
  }

  Write-Host ""
  Write-Host "Kész. Nyisd meg: http://localhost:$Port  (adatok: $DataDir)"
}
finally {
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
