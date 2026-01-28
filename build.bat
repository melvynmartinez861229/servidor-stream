@echo off
echo ============================================
echo   SRT Server Stream - Script de Compilacion
echo ============================================
echo.

:: Verificar que Wails esta instalado
where wails >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Wails CLI no esta instalado.
    echo Instalar con: go install github.com/wailsapp/wails/v2/cmd/wails@latest
    pause
    exit /b 1
)

:: Verificar que Node.js esta instalado
where node >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Node.js no esta instalado.
    echo Descargar desde: https://nodejs.org/
    pause
    exit /b 1
)

:: Verificar que Go esta instalado
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go no esta instalado.
    echo Descargar desde: https://go.dev/
    pause
    exit /b 1
)

echo [INFO] Verificaciones completadas
echo.

:: Instalar dependencias del frontend
echo [INFO] Instalando dependencias del frontend...
cd frontend
call npm install
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Error instalando dependencias del frontend
    cd ..
    pause
    exit /b 1
)
cd ..
echo [OK] Dependencias del frontend instaladas
echo.

:: Descargar dependencias de Go
echo [INFO] Descargando dependencias de Go...
go mod download
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Error descargando dependencias de Go
    pause
    exit /b 1
)
go mod tidy
echo [OK] Dependencias de Go instaladas
echo.

:: Compilar la aplicacion
echo [INFO] Compilando aplicacion...
wails build -clean
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Error compilando la aplicacion
    pause
    exit /b 1
)

echo.
echo ============================================
echo   Compilacion completada exitosamente!
echo ============================================
echo.
echo El ejecutable se encuentra en: build\bin\servidor-stream.exe
echo.
pause
