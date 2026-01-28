# SRT Server Stream

Sistema de gestión de streams SRT para producción audiovisual con Go Wails.

![SRT Server Stream](docs/screenshot.png)

## Características

- 🎬 **Gestión de múltiples canales SRT** - Administra varios streams simultáneamente
- 🔌 **WebSocket Server** - Comunicación bidireccional con clientes Aximmetry
- 👁️ **Previsualizaciones en tiempo real** - Miniaturas de baja calidad para monitoreo
- ⚡ **Integración FFmpeg** - Generación robusta de streams SRT
- 🔄 **Reinicio automático** - Recuperación ante fallos
- 🎨 **Interfaz moderna** - UI intuitiva para operación en tiempo real

## Requisitos

- **Go** 1.21 o superior
- **Node.js** 18 o superior
- **FFmpeg** con soporte SRT (versión estándar incluye soporte)
- **Wails CLI** v2.7+
- **Windows** 10/11

## Instalación

### 1. Instalar Wails CLI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 2. Clonar el repositorio

```bash
git clone https://github.com/tu-usuario/srt-server-stream.git
cd srt-server-stream
```

### 3. Instalar dependencias

```bash
# Backend Go
go mod download

# Frontend
cd frontend
npm install
cd ..
```

### 4. Desarrollo

```bash
wails dev
```

### 5. Compilar para producción

```bash
wails build
```

El ejecutable se generará en `build/bin/`.

## Configuración de FFmpeg con SRT

FFmpeg moderno incluye soporte SRT por defecto. El protocolo SRT (Secure Reliable Transport) ofrece:

- Baja latencia para streaming en tiempo real
- Corrección de errores (ARQ)
- Encriptación AES
- Atraviesa firewalls fácilmente

### Verificar soporte SRT

```bash
ffmpeg -protocols | grep srt
```

## Uso

### Interfaz Principal

1. **Agregar Canal**: Click en "Nuevo" para crear un canal
2. **Configurar**: Establecer nombre y nombre del stream SRT
3. **Iniciar Stream**: El stream inicia cuando Aximmetry solicita un video
4. **Monitorear**: Ver previsualizaciones y logs en tiempo real

### Comunicación WebSocket

La aplicación expone un servidor WebSocket en el puerto configurado (default: 8765).

#### Protocolo de Mensajes

**Solicitar lista de canales:**
```json
{
  "action": "list_channels"
}
```

**Reproducir video (flujo principal):**
```json
{
  "action": "play_video",
  "filePath": "C:\\Videos\\video.mp4",
  "channelId": "uuid-del-canal"
}
```

**Iniciar reproducción:**
```json
{
  "action": "play",
  "channelId": "uuid-del-canal",
  "filePath": "C:\\Videos\\video.mp4"
}
```

**Detener reproducción:**
```json
{
  "action": "stop",
  "channelId": "uuid-del-canal"
}
```

**Consultar estado:**
```json
{
  "action": "status",
  "channelId": "uuid-del-canal"
}
```

### Integración con Aximmetry

1. En Aximmetry, crear un módulo de WebSocket cliente
2. Conectar a `ws://ip-servidor:8765/ws`
3. Enviar comandos JSON para controlar streams
4. Recibir streams SRT con la URL: `srt://ip-servidor:puerto`

## Estructura del Proyecto

```
srt-server-stream/
├── main.go                 # Punto de entrada
├── wails.json             # Configuración Wails
├── go.mod                 # Dependencias Go
├── internal/
│   ├── app/
│   │   └── app.go         # Lógica principal de la aplicación
│   ├── channel/
│   │   └── channel.go     # Gestión de canales
│   ├── config/
│   │   └── config.go      # Configuración
│   ├── ffmpeg/
│   │   └── manager.go     # Gestión de procesos FFmpeg
│   ├── preview/
│   │   └── preview.go     # Generación de previews
│   └── websocket/
│       └── server.go      # Servidor WebSocket
├── frontend/
│   ├── index.html         # HTML principal
│   ├── package.json       # Dependencias frontend
│   └── src/
│       ├── js/
│       │   └── main.js    # JavaScript principal
│       └── styles/
│           └── main.css   # Estilos CSS
└── docs/
    └── aximmetry-client.js  # Ejemplo de cliente
```

## Configuración

La configuración se guarda en:
- **Windows**: `%APPDATA%/servidor-stream/config.json`

### Parámetros Configurables

| Parámetro | Descripción | Default |
|-----------|-------------|---------|
| `webSocketPort` | Puerto del servidor WebSocket | 8765 |
| `ffmpegPath` | Ruta al ejecutable FFmpeg | "ffmpeg" |
| `autoRestart` | Reinicio automático ante fallos | true |
| `defaultVideoBitrate` | Bitrate de video | "10M" |
| `defaultAudioBitrate` | Bitrate de audio | "192k" |
| `defaultFrameRate` | Frame rate por defecto | 30 |
| `srtPrefix` | Prefijo para nombres de stream | "SRT_SERVER_" |
| `previewConfig.width` | Ancho de previews | 320 |
| `previewConfig.height` | Alto de previews | 180 |
| `previewConfig.quality` | Calidad JPEG (%) | 60 |
| `previewConfig.updateIntervalMs` | Intervalo de actualización | 2000 |

## API REST

Además de WebSockets, hay endpoints REST disponibles:

- `GET /health` - Estado del servidor
- `GET /api/channels` - Lista de canales

## Desarrollo

### Comandos útiles

```bash
# Modo desarrollo con hot-reload
wails dev

# Compilar para Windows
wails build -platform windows/amd64

# Compilar con debug
wails build -debug

# Generar bindings
wails generate module
```

### Agregar nuevas funcionalidades

1. Agregar métodos en `internal/app/app.go`
2. Los métodos públicos se exponen automáticamente al frontend
3. Usar `runtime.EventsEmit()` para eventos en tiempo real

## Solución de Problemas

### FFmpeg no encontrado
Asegúrate de que FFmpeg está en el PATH o configura la ruta completa en Configuración.

### Stream SRT no conecta
1. Verificar que el receptor está apuntando al puerto correcto
2. Comprobar firewall de Windows (abrir puertos SRT: 9000+)
3. Verificar que la IP del servidor es accesible

### Previews no se actualizan
1. Verificar que el archivo de video existe
2. Comprobar logs de errores
3. Ajustar intervalo de actualización

## Contribuir

1. Fork el repositorio
2. Crear rama feature (`git checkout -b feature/nueva-funcionalidad`)
3. Commit cambios (`git commit -am 'Agregar funcionalidad'`)
4. Push a la rama (`git push origin feature/nueva-funcionalidad`)
5. Crear Pull Request

## Licencia

MIT License - ver [LICENSE](LICENSE) para detalles.

## Contacto

- Crear un [Issue](https://github.com/tu-usuario/srt-server-stream/issues) para reportar bugs
- Pull requests bienvenidos

---

Desarrollado para producción audiovisual profesional con ❤️
