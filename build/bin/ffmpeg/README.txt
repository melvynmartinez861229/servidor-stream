================================================================================
                         FFMPEG PARA NDI SERVER STREAM
================================================================================

FFmpeg v8.0.1 ya está instalado en esta carpeta.

ARCHIVOS INCLUIDOS:
-------------------
  - ffmpeg.exe   (94.7 MB) - Codificador de video
  - ffprobe.exe  (94.5 MB) - Analizador de video

NOTA SOBRE NDI:
---------------
Esta versión de FFmpeg NO incluye soporte NDI nativo (libndi_newtek)
debido a restricciones de licencia de NewTek.

OPCIONES PARA SALIDA NDI:
-------------------------

Opción 1: NDI Tools (RECOMENDADO)
   - Descarga "NDI Tools" gratis de: https://ndi.video/tools/
   - Incluye "NDI Virtual Input" que puede recibir cualquier fuente
   - El servidor puede enviar via SRT y NDI Tools lo convierte

Opción 2: OBS + NDI Plugin
   - Usa OBS Studio con el plugin "obs-ndi"
   - Recibe el stream SRT y lo retransmite como NDI

Opción 3: Compilar FFmpeg con NDI
   - Requiere el NDI SDK de NewTek
   - Más información: https://ndi.video/sdk/

VERIFICAR INSTALACIÓN:
----------------------
Ejecuta en terminal: ffmpeg -version
Deberías ver: ffmpeg version 8.0.1

================================================================================

