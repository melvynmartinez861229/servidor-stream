/**
 * Cliente WebSocket para NDI Server Stream
 * Ejemplo de integración con Aximmetry Composer
 * 
 * Este archivo muestra cómo conectar desde Aximmetry u otros clientes
 * al servidor NDI Stream para controlar la reproducción de videos.
 */

class NDIServerClient {
    constructor(serverUrl = 'ws://localhost:8765/ws') {
        this.serverUrl = serverUrl;
        this.ws = null;
        this.clientName = 'Aximmetry_Client';
        this.isConnected = false;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 3000;
        this.messageHandlers = new Map();
        this.pendingRequests = new Map();
        this.requestId = 0;
    }

    /**
     * Conectar al servidor NDI Stream
     * @param {string} clientName - Nombre identificativo del cliente
     * @returns {Promise} - Resuelve cuando la conexión está establecida
     */
    connect(clientName = this.clientName) {
        return new Promise((resolve, reject) => {
            this.clientName = clientName;
            const url = `${this.serverUrl}?name=${encodeURIComponent(clientName)}`;
            
            console.log(`[NDI Client] Conectando a ${url}...`);
            
            this.ws = new WebSocket(url);
            
            this.ws.onopen = () => {
                console.log('[NDI Client] Conexión establecida');
                this.isConnected = true;
                this.reconnectAttempts = 0;
                resolve();
            };
            
            this.ws.onclose = (event) => {
                console.log(`[NDI Client] Conexión cerrada: ${event.code} - ${event.reason}`);
                this.isConnected = false;
                this.handleDisconnect();
            };
            
            this.ws.onerror = (error) => {
                console.error('[NDI Client] Error de conexión:', error);
                reject(error);
            };
            
            this.ws.onmessage = (event) => {
                this.handleMessage(event.data);
            };
        });
    }

    /**
     * Desconectar del servidor
     */
    disconnect() {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this.isConnected = false;
    }

    /**
     * Manejar desconexión e intentar reconectar
     */
    handleDisconnect() {
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
            this.reconnectAttempts++;
            console.log(`[NDI Client] Intentando reconectar (${this.reconnectAttempts}/${this.maxReconnectAttempts})...`);
            
            setTimeout(() => {
                this.connect(this.clientName).catch(err => {
                    console.error('[NDI Client] Error en reconexión:', err);
                });
            }, this.reconnectDelay);
        } else {
            console.error('[NDI Client] Máximo de intentos de reconexión alcanzado');
            this.emit('connectionFailed');
        }
    }

    /**
     * Manejar mensaje entrante
     * @param {string} data - Datos del mensaje
     */
    handleMessage(data) {
        try {
            const message = JSON.parse(data);
            console.log('[NDI Client] Mensaje recibido:', message);
            
            // Emitir evento según la acción
            this.emit(message.action, message);
            
            // Manejar respuestas a requests pendientes
            if (message.requestId && this.pendingRequests.has(message.requestId)) {
                const { resolve, reject } = this.pendingRequests.get(message.requestId);
                this.pendingRequests.delete(message.requestId);
                
                if (message.success) {
                    resolve(message.data);
                } else {
                    reject(new Error(message.error || 'Error desconocido'));
                }
            }
        } catch (error) {
            console.error('[NDI Client] Error parseando mensaje:', error);
        }
    }

    /**
     * Enviar mensaje al servidor
     * @param {Object} message - Mensaje a enviar
     * @returns {Promise} - Resuelve con la respuesta del servidor
     */
    send(message) {
        return new Promise((resolve, reject) => {
            if (!this.isConnected) {
                reject(new Error('No conectado al servidor'));
                return;
            }

            const requestId = ++this.requestId;
            message.requestId = requestId;
            
            this.pendingRequests.set(requestId, { resolve, reject });
            
            // Timeout para la respuesta
            setTimeout(() => {
                if (this.pendingRequests.has(requestId)) {
                    this.pendingRequests.delete(requestId);
                    reject(new Error('Timeout esperando respuesta'));
                }
            }, 10000);

            this.ws.send(JSON.stringify(message));
        });
    }

    /**
     * Registrar handler para eventos
     * @param {string} event - Nombre del evento
     * @param {Function} handler - Función handler
     */
    on(event, handler) {
        if (!this.messageHandlers.has(event)) {
            this.messageHandlers.set(event, []);
        }
        this.messageHandlers.get(event).push(handler);
    }

    /**
     * Emitir evento
     * @param {string} event - Nombre del evento
     * @param {*} data - Datos del evento
     */
    emit(event, data) {
        const handlers = this.messageHandlers.get(event);
        if (handlers) {
            handlers.forEach(handler => handler(data));
        }
    }

    // ==================== API de Control ====================

    /**
     * MÉTODO PRINCIPAL: Solicitar reproducción de un video
     * Aximmetry envía la ruta del video que quiere ver
     * El servidor responde con el nombre del stream NDI
     * 
     * @param {string} filePath - Ruta completa del video a reproducir
     * @param {string} [channelId] - ID del canal (opcional, se asigna automáticamente)
     * @returns {Promise<Object>} - { channelId, ndiStreamName, filePath }
     * 
     * @example
     * const result = await client.playVideo('C:\\Videos\\intro.mp4');
     * console.log('Stream NDI disponible:', result.ndiStreamName);
     * // Usar result.ndiStreamName como fuente NDI en Aximmetry
     */
    async playVideo(filePath, channelId = null) {
        const message = {
            action: 'play_video',
            filePath: filePath
        };
        
        if (channelId) {
            message.channelId = channelId;
        }
        
        const response = await this.send(message);
        return response;
    }

    /**
     * Obtener lista de canales disponibles
     * @returns {Promise<Array>} - Lista de canales
     */
    async getChannels() {
        const response = await this.send({
            action: 'list_channels'
        });
        return response;
    }

    /**
     * Obtener estado de un canal específico
     * @param {string} channelId - ID del canal
     * @returns {Promise<Object>} - Estado del canal
     */
    async getChannelStatus(channelId) {
        const response = await this.send({
            action: 'status',
            channelId: channelId
        });
        return response;
    }

    /**
     * Obtener estado de todos los canales
     * @returns {Promise<Array>} - Estado de todos los canales
     */
    async getAllStatus() {
        const response = await this.send({
            action: 'status'
        });
        return response;
    }

    /**
     * Iniciar reproducción en un canal existente
     * @param {string} channelId - ID del canal
     * @param {string} [filePath] - Ruta del archivo (opcional)
     * @returns {Promise<Object>} - Resultado de la operación
     */
    async play(channelId, filePath = null) {
        const message = {
            action: 'play',
            channelId: channelId
        };
        
        if (filePath) {
            message.filePath = filePath;
        }
        
        const response = await this.send(message);
        return response;
    }

    /**
     * Detener reproducción en un canal
     * @param {string} channelId - ID del canal
     * @returns {Promise<Object>} - Resultado de la operación
     */
    async stop(channelId) {
        const response = await this.send({
            action: 'stop',
            channelId: channelId
        });
        return response;
    }

    /**
     * Listar archivos de video disponibles para un canal
     * @param {string} channelId - ID del canal
     * @returns {Promise<Array>} - Lista de archivos
     */
    async listFiles(channelId) {
        const response = await this.send({
            action: 'list_files',
            channelId: channelId
        });
        return response;
    }
}

// ==================== Ejemplo de Uso para Aximmetry ====================

/*
// =====================================================
// FLUJO PRINCIPAL: Aximmetry solicita un video al servidor
// =====================================================

// 1. Crear instancia del cliente
const client = new NDIServerClient('ws://192.168.1.100:8765/ws');

// 2. Registrar handlers de eventos
client.on('connected', (data) => {
    console.log('Conectado al servidor NDI:', data);
});

client.on('play_started', (data) => {
    console.log('Video iniciado:', data);
    console.log('Stream NDI disponible en:', data.ndiStreamName);
    // ¡Usar data.ndiStreamName como fuente NDI en Aximmetry!
});

client.on('play_stopped', (data) => {
    console.log('Reproducción detenida:', data);
});

// 3. Conectar y solicitar videos
async function main() {
    try {
        // Conectar al servidor
        await client.connect('Aximmetry_Studio_1');
        
        // =====================================================
        // SOLICITAR UN VIDEO - Este es el flujo principal
        // =====================================================
        // Aximmetry envía la ruta del video que quiere ver
        // El servidor responde con el nombre del stream NDI
        
        const result = await client.playVideo('C:\\Videos\\intro.mp4');
        
        console.log('========================================');
        console.log('VIDEO SOLICITADO CORRECTAMENTE');
        console.log('Stream NDI:', result.ndiStreamName);
        console.log('Archivo:', result.filePath);
        console.log('========================================');
        
        // Ahora en Aximmetry:
        // 1. Agregar una fuente NDI
        // 2. Buscar el stream con nombre: result.ndiStreamName
        // 3. El video estará disponible para usar
        
        // =====================================================
        // CAMBIAR A OTRO VIDEO
        // =====================================================
        // Simplemente solicitar otro video en cualquier momento
        
        // await client.playVideo('C:\\Videos\\outro.mp4');
        // El stream NDI se actualiza automáticamente con el nuevo video
        
        // =====================================================
        // DETENER REPRODUCCIÓN
        // =====================================================
        // await client.stop(result.channelId);
        
    } catch (error) {
        console.error('Error:', error);
    }
}

main();

// =====================================================
// EJEMPLO: Cambiar videos dinámicamente
// =====================================================
async function playlistExample() {
    await client.connect('Aximmetry_Playlist');
    
    const videos = [
        'C:\\Videos\\intro.mp4',
        'C:\\Videos\\contenido.mp4',
        'C:\\Videos\\outro.mp4'
    ];
    
    for (const video of videos) {
        const result = await client.playVideo(video);
        console.log(`Reproduciendo: ${video} -> NDI: ${result.ndiStreamName}`);
        
        // Esperar 10 segundos antes del siguiente video
        await new Promise(r => setTimeout(r, 10000));
    }
}
*/

// Exportar para uso en módulos
if (typeof module !== 'undefined' && module.exports) {
    module.exports = NDIServerClient;
}

// Exportar para uso global en navegador
if (typeof window !== 'undefined') {
    window.NDIServerClient = NDIServerClient;
}
