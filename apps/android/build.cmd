@echo off
rem Kinora Android APK – Windows wrapper. Usage: build.cmd [--release] [--install]
cd /d "%~dp0"
if not exist "..\node_modules" (pushd .. & call npm install & popd)
node build.mjs %*
