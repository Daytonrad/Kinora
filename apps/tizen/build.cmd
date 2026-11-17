@echo off
rem Kinora Samsung Tizen TV app - Windows wrapper. Usage: build.cmd [--install]
cd /d "%~dp0"
if not exist "..\node_modules" (pushd .. & call npm install & popd)
node build.mjs %*
