@echo off
chcp 65001 >nul
setlocal

set VERSION=%~1
if "%VERSION%"=="" set VERSION=%DATE:~6,4%.%DATE:~3,2%.%DATE:~0,2%-%TIME:~0,2%%TIME:~3,2%%TIME:~6,2%
set VERSION=%VERSION: =0%
set BUILD_DIR=build
set LDFLAGS=-ldflags="-X main.version=%VERSION% -s -w"

echo Building ppdftb tools...
echo Version: %VERSION%
echo Output: %BUILD_DIR%\
echo.

if not exist %BUILD_DIR% mkdir %BUILD_DIR%

go build %LDFLAGS% -o %BUILD_DIR%\aconv.exe .\cmd\aconv\ || echo FAIL: aconv
go build %LDFLAGS% -o %BUILD_DIR%\mpdf.exe  .\cmd\mpdf\  || echo FAIL: mpdf
go build %LDFLAGS% -o %BUILD_DIR%\pnpdf.exe .\cmd\pnpdf\ || echo FAIL: pnpdf
go build %LDFLAGS% -o %BUILD_DIR%\toc.exe   .\cmd\toc\   || echo FAIL: toc
go build %LDFLAGS% -o %BUILD_DIR%\wconv.exe .\cmd\wconv\ || echo FAIL: wconv
go build %LDFLAGS% -o %BUILD_DIR%\engine.exe  .\cmd\engine\  || echo FAIL: engine
go build %LDFLAGS% -o %BUILD_DIR%\engine2.exe .\cmd\engine2\ || echo FAIL: engine2

echo.
echo Done. Files in %BUILD_DIR%\:
dir /B %BUILD_DIR%\*.exe

endlocal
pause
