# Kinora Android APK – PowerShell wrapper. Usage: .\build.ps1 [--release] [--install]
Set-Location $PSScriptRoot
if (-not (Test-Path ..\node_modules)) { Push-Location ..; npm install; Pop-Location }
node build.mjs @args
exit $LASTEXITCODE
