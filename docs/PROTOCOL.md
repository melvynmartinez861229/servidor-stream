# Protocolo WebSocket - NDI Server Stream

Este documento describe el protocolo de comunicación WebSocket entre el servidor NDI Stream y los clientes (como Aximmetry Composer).

## Flujo Principal

El flujo de trabajo típico es:

1. **Aximmetry (cliente)** se conecta al servidor WebSocket
2. **Aximmetry** envía la ruta del video que quiere reproducir
3. **NDI Server** recibe la solicitud e inicia FFmpeg con ese video
4. **NDI Server** responde con el nombre del stream NDI
5. **Aximmetry** consume el stream NDI usando el nombre proporcionado

```
┌─────────────┐                     ┌─────────────┐
│  Aximmetry  │                     │ NDI Server  │
└──────┬──────┘                     └──────┬──────┘
       │                                   │
       │  1. Conectar WebSocket            │
       │──────────────────────────────────>│
       │                                   │
       │  2. play_video {filePath: "..."}  │
       │──────────────────────────────────>│
       │                                   │
       │  3. {ndiStreamName: "NDI_XXX"}    │
       │<──────────────────────────────────│
       │                                   │
       │  4. Recibir stream NDI "NDI_XXX"  │
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

Aximmetry envía la ruta del video que quiere reproducir, el servidor lo procesa y devuelve el nombre del stream NDI.

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
    "ndiStreamName": "NDI_SERVER_abc123",
    "filePath": "C:\\Videos\\intro.mp4",
    "message": "Video disponible en stream NDI: NDI_SERVER_abc123"
  }
}
```

**Uso en Aximmetry:**
1. Enviar `play_video` con la ruta del video
2. Recibir `ndiStreamName` en la respuesta
3. Usar ese nombre como fuente NDI en Aximmetry

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
      "ndiStreamName": "NDI_CANAL_1",
      "status": "inactive",
      "previewEnabled": true,
      "currentFile": "C:\\Videos\\video.mp4"
    }
  ]
}
```

### 2. status
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
    "ndiStreamName": "NDI_CANAL_1",
    "currentFile": "C:\\Videos\\video.mp4",
    "stats": {
      "framesProcessed": 1500,
      "uptime": "00:01:00"
    }
  }
}
```

### 3. play
Inicia la reproducción de un video en un canal.

**Request:**
```json
{
  "action": "play",
  "channelId": "uuid-del-canal",
  "filePath": "C:\\Videos\\otro-video.mp4"  // Opcional
}
```

**Response:**
```json
{
  "success": true,
  "action": "play_started",
  "data": {
    "channelId": "uuid-del-canal",
    "ndiStreamName": "NDI_CANAL_1",
    "filePath": "C:\\Videos\\otro-video.mp4"
  }
}
```

### 4. stop
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

### 5. list_files
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
  "message": "Conectado al servidor NDI Stream",
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
    "currentFile": "C:\\Videos\\video.mp4"
  }
}
```

## Estados de Canal

| Estado | Descripción |
|--------|-------------|
| `inactive` | Canal configurado pero sin stream activo |
| `active` | Stream NDI activo y transmitiendo |
| `starting` | Iniciando proceso FFmpeg |
| `stopping` | Deteniendo proceso FFmpeg |
| `error` | Error en el proceso de streaming |

## Códigos de Error

| Código | Descripción |
|--------|-------------|
| `invalid_message` | Mensaje JSON mal formado |
| `unknown_action` | Acción no reconocida |
| `channel_not_found` | Canal no existe |
| `play_error` | Error iniciando reproducción |
| `stop_error` | Error deteniendo reproducción |
| `list_error` | Error listando archivos |

## Ejemplo de Flujo Completo

```javascript
// 1. Conectar
ws.connect('ws://192.168.1.100:8765/ws?name=Aximmetry_1');

// 2. Recibir confirmación de conexión
// <- { "action": "connected", "data": { "clientId": "abc-123" } }

// 3. Listar canales disponibles
ws.send({ "action": "list_channels" });
// <- { "action": "channels_list", "data": [...] }

// 4. Iniciar reproducción
ws.send({ "action": "play", "channelId": "canal-1" });
// <- { "action": "play_started", "data": { "ndiStreamName": "NDI_CANAL_1" } }

// 5. El stream NDI está ahora disponible para recibir en Aximmetry
// Usar el nombre "NDI_CANAL_1" como fuente NDI

// 6. Detener cuando termine
ws.send({ "action": "stop", "channelId": "canal-1" });
// <- { "action": "play_stopped", "data": { "channelId": "canal-1" } }
```

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
