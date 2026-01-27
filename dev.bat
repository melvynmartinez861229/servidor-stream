@echo off
echo ============================================
echo   NDI Server Stream - Modo Desarrollo
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

echo [INFO] Iniciando en modo desarrollo...
echo [INFO] La aplicacion se recargara automaticamente al detectar cambios
echo [INFO] Presiona Ctrl+C para detener
echo.

wails dev
