# Protocolo WebSocket - SRT Server Stream

Este documento describe el protocolo de comunicación WebSocket entre el servidor SRT Stream y los clientes (como Aximmetry Composer).

## Flujo Principal

El flujo de trabajo típico es:

1. **Aximmetry (cliente)** se conecta al servidor WebSocket
2. **Aximmetry** envía la ruta del video que quiere reproducir
3. **SRT Server** recibe la solicitud e inicia FFmpeg con ese video
4. **SRT Server** responde con la URL del stream SRT
5. **Aximmetry** consume el stream SRT usando la URL proporcionada

```
┌─────────────┐                     ┌─────────────┐
│  Aximmetry  │                     │ SRT Server  │
└──────┬──────┘                     └──────┬──────┘
       │                                   │
       │  1. Conectar WebSocket            │
       │──────────────────────────────────>│
       │                                   │
       │  2. play_video {filePath: "..."}  │
       │──────────────────────────────────>│
       │                                   │
       │  3. {srtUrl: "srt://IP:9000"}     │
       │<──────────────────────────────────│
       │                                   │
       │  4. Recibir stream SRT            │
       │<==================================│
       │                                   │
```

## Conexión

### URL de Conexión
```
ws://{host}:{port}/ws?name={clientName}
```

- **host**: IP o hostname del servidor (default: localhost)
- **port**: Puerto WebSocket (default: 8765)
- **clientName**: Nombre identificativo del cliente (opcional)

### Ejemplo
```
ws://192.168.1.100:8765/ws?name=Aximmetry_Studio_1
```

## Formato de Mensajes

Todos los mensajes son objetos JSON con la siguiente estructura base:

### Request (Cliente → Servidor)
```json
{
  "action": "string",           // Acción a realizar
  "clientId": "string",         // ID del cliente (opcional)
  "channelId": "string",        // ID del canal (cuando aplica)
  "filePath": "string",         // Ruta del archivo (cuando aplica)
  "parameters": {}              // Parámetros adicionales (opcional)
}
```

### Response (Servidor → Cliente)
```json
{
  "success": true,              // Resultado de la operación
  "action": "string",           // Acción que generó la respuesta
  "message": "string",          // Mensaje descriptivo (opcional)
  "data": {},                   // Datos de respuesta
  "error": "string"             // Mensaje de error (si success=false)
}
```

## Acciones Disponibles

### 1. play_video (Acción Principal)
**Esta es la acción principal que usa Aximmetry para solicitar videos.**

Aximmetry envía la ruta del video que quiere reproducir, el servidor lo procesa y devuelve la URL del stream SRT.

**Request:**
```json
{
  "action": "play_video",
  "filePath": "C:\\Videos\\intro.mp4",
  "channelId": "opcional-uuid"
}
```

- `filePath` (requerido): Ruta completa del video a reproducir
- `channelId` (opcional): Si se omite, el servidor asigna/crea un canal automáticamente

**Response:**
```json
{
  "success": true,
  "action": "play_started",
  "data": {
    "channelId": "uuid-asignado",
    "streamName": "SRT_SERVER_abc123",
    "srtPort": 9000,
    "srtUrl": "srt://192.168.1.100:9000",
    "filePath": "C:\\Videos\\intro.mp4",
    "message": "Video disponible en: srt://192.168.1.100:9000"
  }
}
```

**Uso en Aximmetry:**
1. Enviar `play_video` con la ruta del video
2. Recibir `srtUrl` en la respuesta
3. Usar esa URL como fuente SRT en Aximmetry

### 2. list_channels
Obtiene la lista de canales configurados.

**Request:**
```json
{
  "action": "list_channels"
}
```

**Response:**
```json
{
  "success": true,
  "action": "channels_list",
  "data": [
    {
      "id": "uuid-del-canal",
      "label": "Canal Principal",
      "videoPath": "C:\\Videos\\video.mp4",
      "srtStreamName": "SRT_CANAL_1",
      "srtPort": 9000,
      "status": "inactive",
      "previewEnabled": true,
      "currentFile": "C:\\Videos\\video.mp4"
    }
  ]
}
```

### 3. status
Obtiene el estado de un canal específico o todos los canales.

**Request (canal específico):**
```json
{
  "action": "status",
  "channelId": "uuid-del-canal"
}
```

**Request (todos los canales):**
```json
{
  "action": "status"
}
```

**Response:**
```json
{
  "success": true,
  "action": "channel_status",
  "data": {
    "id": "uuid-del-canal",
    "label": "Canal Principal",
    "status": "active",
    "srtStreamName": "SRT_CANAL_1",
    "srtPort": 9000,
    "currentFile": "C:\\Videos\\video.mp4",
    "stats": {
      "framesProcessed": 1500,
      "uptime": "00:01:00"
    }
  }
}
```

### 4. play
Inicia la reproducción de un video en un canal.

**Request:**
```json
{
  "action": "play",
  "channelId": "uuid-del-canal",
  "filePath": "C:\\Videos\\otro-video.mp4"
}
```

**Response:**
```json
{
  "success": true,
  "action": "play_started",
  "data": {
    "channelId": "uuid-del-canal",
    "streamName": "SRT_CANAL_1",
    "srtPort": 9000,
    "srtUrl": "srt://192.168.1.100:9000",
    "filePath": "C:\\Videos\\otro-video.mp4"
  }
}
```

### 5. stop
Detiene la reproducción de un canal.

**Request:**
```json
{
  "action": "stop",
  "channelId": "uuid-del-canal"
}
```

**Response:**
```json
{
  "success": true,
  "action": "play_stopped",
  "data": {
    "channelId": "uuid-del-canal"
  }
}
```

### 6. list_files
Lista los archivos de video disponibles en el directorio del canal.

**Request:**
```json
{
  "action": "list_files",
  "channelId": "uuid-del-canal"
}
```

**Response:**
```json
{
  "success": true,
  "action": "files_list",
  "data": [
    "C:\\Videos\\video1.mp4",
    "C:\\Videos\\video2.mp4",
    "C:\\Videos\\demo.avi"
  ]
}
```

## Eventos del Servidor (Push)

El servidor puede enviar eventos sin solicitud previa:

### Mensaje de Bienvenida
Enviado al conectarse:
```json
{
  "success": true,
  "action": "connected",
  "message": "Conectado al servidor SRT Stream",
  "data": {
    "clientId": "uuid-asignado",
    "name": "Aximmetry_Studio_1"
  }
}
```

### Actualización de Estado
Cuando cambia el estado de un canal:
```json
{
  "success": true,
  "action": "channel_status_update",
  "data": {
    "channelId": "uuid-del-canal",
    "status": "active",
    "srtPort": 9000,
    "currentFile": "C:\\Videos\\video.mp4"
  }
}
```

## Estados de Canal

| Estado | Descripción |
|--------|-------------|
| `inactive` | Canal configurado pero sin stream activo |
| `active` | Stream SRT activo y transmitiendo |
| `starting` | Iniciando proceso FFmpeg |
| `stopping` | Deteniendo proceso FFmpeg |
| `error` | Error en el proceso de streaming |

## Códigos de Error

| Código | Descripción |
|--------|-------------|
| `invalid_message` | Mensaje JSON mal formado |
| `unknown_action` | Acción no reconocida |
| `channel_not_found` | Canal no existe |
| `file_not_found` | Archivo de video no encontrado |
| `play_error` | Error iniciando reproducción |
| `stop_error` | Error deteniendo reproducción |
| `list_error` | Error listando archivos |

## Ejemplo de Flujo Completo

```javascript
// 1. Conectar
ws.connect('ws://192.168.1.100:8765/ws?name=Aximmetry_1');

// 2. Recibir confirmación de conexión
// <- { "action": "connected", "data": { "clientId": "abc-123" } }

// 3. Solicitar reproducción de video (flujo principal)
ws.send({ "action": "play_video", "filePath": "C:\\Videos\\intro.mp4" });
// <- { "action": "play_started", "data": { "srtUrl": "srt://192.168.1.100:9000", ... } }

// 4. El stream SRT está ahora disponible para recibir en Aximmetry
// Usar la URL "srt://192.168.1.100:9000" como fuente

// 5. Detener cuando termine
ws.send({ "action": "stop", "channelId": "canal-asignado" });
// <- { "action": "play_stopped", "data": { "channelId": "canal-asignado" } }
```

## Puertos SRT

- Cada canal tiene su propio puerto SRT único
- El primer canal usa el puerto 9000
- Los siguientes canales usan puertos incrementales (9001, 9002, etc.)
- Asegúrese de abrir estos puertos en el firewall

## Consideraciones de Implementación

### Reconexión
- Implementar lógica de reconexión automática
- Usar backoff exponencial (ej: 1s, 2s, 4s, 8s)
- Máximo 5 intentos antes de reportar fallo

### Heartbeat
- El servidor envía PING cada 30 segundos
- El cliente debe responder con PONG
- Conexión cerrada si no hay respuesta en 60 segundos

### Timeouts
- Timeout de respuesta recomendado: 10 segundos
- Para operaciones largas (play), esperar confirmación

### Múltiples Clientes
- Múltiples clientes pueden conectarse simultáneamente
- Cada cliente tiene un ID único
- Los cambios de estado se notifican a todos los clientes

### Latencia SRT
- El servidor usa latency=200000 (200ms) por defecto
- Ajustable según las necesidades de la red
- Menor latencia = menos buffer, más sensible a pérdidas
