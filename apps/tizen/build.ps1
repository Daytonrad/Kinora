# Kinora tizen TV app - PowerShell wrapper. Usage: .uild.ps1 [--install]
Set-Location $PSScriptRoot
if (-not (Test-Path ..
ode_modules)) { Push-Location ..; npm install; Pop-Location }
node build.mjs @args
exit $LASTEXITCODE
