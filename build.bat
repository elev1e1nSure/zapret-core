@echo off
setlocal

echo Building zapret-core...
go build -ldflags="-s -w -H windowsgui" -o zapret-core.exe .

if %ERRORLEVEL% neq 0 (
    echo Build failed
    exit /b 1
)

echo Build successful
echo.
echo Creating distribution package...

set VERSION=1.0.0
set DIST_DIR=dist\zapret-core-%VERSION%

if exist dist rmdir /s /q dist
mkdir dist
mkdir "%DIST_DIR%"

copy zapret-core.exe "%DIST_DIR%\"
xcopy /E /I /Y assets "%DIST_DIR%\assets"
xcopy /E /I /Y lists "%DIST_DIR%\lists"
copy README.md "%DIST_DIR%\"
copy README.ru.md "%DIST_DIR%\"

echo Packaging to zip...
powershell -Command "Compress-Archive -Path '%DIST_DIR%' -DestinationPath 'dist\zapret-core-%VERSION%.zip' -Force"

echo.
echo Distribution package created: dist\zapret-core-%VERSION%.zip
echo.
echo Contents:
dir "%DIST_DIR%" /B

endlocal
