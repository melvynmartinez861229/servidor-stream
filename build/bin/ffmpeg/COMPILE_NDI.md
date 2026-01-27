# Compilar FFmpeg con soporte NDI

## Requisitos Previos

### 1. NDI SDK (OBLIGATORIO)
1. Ve a https://ndi.video/download-ndi-sdk/
2. Regístrate gratis (o inicia sesión)
3. Descarga **"NDI SDK for Windows"**
4. Instala el SDK (por defecto en `C:\Program Files\NDI\NDI 6 SDK`)

### 2. MSYS2 (Entorno de compilación)
- Ya se está instalando automáticamente
- Si no se instaló, descarga de: https://www.msys2.org/

---

## Pasos de Compilación

### Paso 1: Abrir MSYS2 MINGW64
```
C:\msys64\mingw64.exe
```

### Paso 2: Instalar dependencias
```bash
pacman -Syu
pacman -S mingw-w64-x86_64-toolchain mingw-w64-x86_64-yasm mingw-w64-x86_64-nasm
pacman -S mingw-w64-x86_64-pkg-config make git diffutils
pacman -S mingw-w64-x86_64-SDL2 mingw-w64-x86_64-x264 mingw-w64-x86_64-x265
```

### Paso 3: Configurar NDI SDK
```bash
# Crear enlace simbólico al NDI SDK
export NDI_SDK_DIR="/c/Program Files/NDI/NDI 6 SDK"

# Verificar que existe
ls "$NDI_SDK_DIR"
```

### Paso 4: Descargar FFmpeg
```bash
cd ~
git clone https://git.ffmpeg.org/ffmpeg.git ffmpeg-ndi
cd ffmpeg-ndi
```

### Paso 5: Configurar FFmpeg con NDI
```bash
./configure \
    --prefix=/usr/local \
    --enable-gpl \
    --enable-nonfree \
    --enable-libndi_newtek \
    --extra-cflags="-I$NDI_SDK_DIR/Include" \
    --extra-ldflags="-L$NDI_SDK_DIR/Lib/x64" \
    --enable-libx264 \
    --enable-libx265
```

### Paso 6: Compilar
```bash
make -j$(nproc)
```

### Paso 7: Copiar resultado
Los ejecutables estarán en:
- `~/ffmpeg-ndi/ffmpeg.exe`
- `~/ffmpeg-ndi/ffprobe.exe`

Cópialos a la carpeta `ffmpeg/` del proyecto.

---

## Alternativa Rápida: Usar builds de la comunidad

Busca en GitHub: "ffmpeg ndi windows build"

Algunos repositorios ofrecen builds pre-compilados:
- https://github.com/jliljebl/FFmpeg-NDI
- https://github.com/ndi-tv/FFmpeg

---

## Verificar instalación

```cmd
ffmpeg -formats | findstr ndi
```

Deberías ver:
```
 DE libndi_newtek   Network Device Interface (NDI)
```

---

## Notas importantes

- La compilación puede tomar **1-2 horas**
- Necesitas aceptar la licencia de NewTek para usar el NDI SDK
- El archivo `Processing.NDI.Lib.x64.dll` debe estar junto a ffmpeg.exe
