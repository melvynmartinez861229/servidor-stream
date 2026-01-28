/**
 * SRT Server Stream - JavaScript Principal
 * Gestión de la interfaz de usuario y comunicación con el backend Wails
 */

// ==================== Estado Global ====================
const state = {
    channels: [],
    selectedChannel: null,
    logs: [],
    config: null,
    connectedClients: [],
    logFilter: 'all'
};

// ==================== Inicialización ====================
document.addEventListener('DOMContentLoaded', async () => {
    console.log('SRT Server Stream - Inicializando...');
    
    // Configurar eventos de UI
    setupEventListeners();
    
    // Esperar a que Wails esté listo
    if (typeof window.go !== 'undefined') {
        await initializeApp();
    } else {
        // Modo desarrollo sin Wails
        console.log('Modo desarrollo - Wails no disponible');
        showMockData();
    }
});

async function initializeApp() {
    try {
        // Cargar datos iniciales
        await loadChannels();
        await loadConfig();
        await loadConnectedClients();
        
        // Configurar eventos de Wails
        setupWailsEvents();
        
        console.log('Aplicación inicializada correctamente');
    } catch (error) {
        console.error('Error inicializando aplicación:', error);
        showToast('error', 'Error', 'No se pudo inicializar la aplicación');
    }
}

// ==================== Carga de Datos ====================
async function loadChannels() {
    try {
        const channels = await window.go.app.App.GetChannels();
        state.channels = channels || [];
        renderChannelList();
        renderChannelGrid();
        updateChannelCount();
    } catch (error) {
        console.error('Error cargando canales:', error);
    }
}

async function loadConfig() {
    try {
        state.config = await window.go.app.App.GetConfig();
        applyConfig();
    } catch (error) {
        console.error('Error cargando configuración:', error);
    }
}

async function loadConnectedClients() {
    try {
        state.connectedClients = await window.go.app.App.GetConnectedClients();
        updateClientCount();
    } catch (error) {
        console.error('Error cargando clientes:', error);
    }
}

async function loadLogs() {
    try {
        state.logs = await window.go.app.App.GetLogs();
        renderLogs();
    } catch (error) {
        console.error('Error cargando logs:', error);
    }
}

// ==================== Eventos de Wails ====================
function setupWailsEvents() {
    // Canal agregado
    window.runtime.EventsOn('channel:added', (channel) => {
        console.log('[EVENT] channel:added', channel?.id, 'status:', channel?.status);
        const existing = state.channels.findIndex(c => c.id === channel.id);
        if (existing === -1) {
            state.channels.push(channel);
        } else {
            // Preservar el status actual si el nuevo no está definido o es diferente
            const currentStatus = state.channels[existing].status;
            state.channels[existing] = channel;
            if (!channel.status && currentStatus) {
                state.channels[existing].status = currentStatus;
            }
        }
        renderChannelList();
        renderChannelGrid();
        updateChannelCount();
    });
    
    // Canal eliminado
    window.runtime.EventsOn('channel:removed', (channelId) => {
        console.log('[EVENT] channel:removed', channelId);
        state.channels = state.channels.filter(c => c.id !== channelId);
        renderChannelList();
        renderChannelGrid();
        updateChannelCount();
    });
    
    // Canal actualizado
    window.runtime.EventsOn('channel:updated', (channel) => {
        console.log('[EVENT] channel:updated', channel?.id, 'status:', channel?.status);
        if (!channel) return; // Ignorar si es null
        const index = state.channels.findIndex(c => c.id === channel.id);
        if (index !== -1) {
            // Preservar status actual si el canal viene con status inactivo pero el stream sigue
            const currentStatus = state.channels[index].status;
            state.channels[index] = channel;
            renderChannelList();
            // No hacer renderChannelGrid completo - solo actualizar status
            updateChannelCardStatus(channel.id, state.channels[index]);
        }
    });
    
    // Estado del canal
    window.runtime.EventsOn('channel:status', (data) => {
        console.log('[EVENT] channel:status', data?.channelId, 'status:', data?.status, 'event:', data?.event);
        const index = state.channels.findIndex(c => c.id === data.channelId);
        if (index !== -1) {
            // Solo actualizar status si viene definido en el evento
            if (data.status !== undefined && data.status !== null && data.status !== '') {
                state.channels[index].status = data.status;
            }
            if (data.currentFile) {
                state.channels[index].currentFile = data.currentFile;
            }
            // Re-renderizar todo el grid y lista para asegurar sincronización
            renderChannelGrid();
            renderChannelList();
        }
    });
    
    // Nuevo log
    window.runtime.EventsOn('log:new', (entry) => {
        state.logs.push(entry);
        if (state.logs.length > 1000) {
            state.logs.shift();
        }
        appendLogEntry(entry);
    });
    
    // Cliente WebSocket conectado
    window.runtime.EventsOn('client:connected', (client) => {
        console.log('[EVENT] client:connected', client);
        state.connectedClients.push(client);
        updateClientCount();
    });
    
    // Cliente WebSocket desconectado
    window.runtime.EventsOn('client:disconnected', (clientId) => {
        console.log('[EVENT] client:disconnected', clientId);
        state.connectedClients = state.connectedClients.filter(c => c.id !== clientId);
        updateClientCount();
    });
    
    // Warning de FFmpeg (fallback de encoder)
    window.runtime.EventsOn('ffmpeg:warning', (data) => {
        console.log('[EVENT] ffmpeg:warning', data);
        showToast('warning', 'Encoder Fallback', data.message);
    });
}

// ==================== Event Listeners de UI ====================
function setupEventListeners() {
    // Botones principales
    document.getElementById('btnAddChannel')?.addEventListener('click', () => openChannelModal());
    document.getElementById('btnAddChannelEmpty')?.addEventListener('click', () => openChannelModal());
    document.getElementById('btnGetStarted')?.addEventListener('click', () => openChannelModal());
    document.getElementById('btnSettings')?.addEventListener('click', () => openSettingsModal());
    document.getElementById('btnToggleLogs')?.addEventListener('click', toggleLogsPanel);
    document.getElementById('btnStopAll')?.addEventListener('click', stopAllStreams);
    
    // Modal de canal
    document.getElementById('btnCloseChannelModal')?.addEventListener('click', closeChannelModal);
    document.getElementById('btnCancelChannel')?.addEventListener('click', closeChannelModal);
    document.getElementById('btnSaveChannel')?.addEventListener('click', saveChannel);
    document.querySelector('#channelModal .modal-overlay')?.addEventListener('click', closeChannelModal);
    
    // Modal de configuración
    document.getElementById('btnCloseSettingsModal')?.addEventListener('click', closeSettingsModal);
    document.getElementById('btnCancelSettings')?.addEventListener('click', closeSettingsModal);
    document.getElementById('btnSaveSettings')?.addEventListener('click', saveSettings);
    document.getElementById('btnSelectTestPattern')?.addEventListener('click', selectTestPatternPath);
    document.querySelector('#settingsModal .modal-overlay')?.addEventListener('click', closeSettingsModal);
    
    // Tabs de configuración
    document.querySelectorAll('.settings-tabs .tab-btn').forEach(btn => {
        btn.addEventListener('click', (e) => switchSettingsTab(e.target.dataset.tab));
    });
    
    // Logs panel
    document.getElementById('btnCloseLogs')?.addEventListener('click', toggleLogsPanel);
    document.getElementById('btnClearLogs')?.addEventListener('click', clearLogs);
    document.getElementById('logFilter')?.addEventListener('change', (e) => {
        state.logFilter = e.target.value;
        renderLogs();
    });
    
    // Modal de confirmación
    document.getElementById('btnConfirmCancel')?.addEventListener('click', closeConfirmModal);
    document.querySelector('#confirmModal .modal-overlay')?.addEventListener('click', closeConfirmModal);
    
    // Teclas de atajo
    document.addEventListener('keydown', handleKeyboardShortcuts);
}

async function selectTestPatternPath() {
    try {
        const path = await window.go.app.App.SelectTestPatternPath();
        if (path) {
            document.getElementById('settingsTestPattern').value = path;
        }
    } catch (error) {
        console.error('Error seleccionando patrón:', error);
    }
}

function handleKeyboardShortcuts(e) {
    // Escape para cerrar modales
    if (e.key === 'Escape') {
        closeAllModals();
    }
    
    // Ctrl+N para nuevo canal
    if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        openChannelModal();
    }
    
    // Ctrl+L para toggle logs
    if (e.ctrlKey && e.key === 'l') {
        e.preventDefault();
        toggleLogsPanel();
    }
}

// ==================== Renderizado ====================
function renderChannelList() {
    const container = document.getElementById('channelList');
    const emptyState = document.getElementById('emptyChannels');
    const welcomeState = document.getElementById('welcomeState');
    
    if (state.channels.length === 0) {
        emptyState.style.display = 'flex';
        welcomeState.classList.remove('hidden');
        return;
    }
    
    emptyState.style.display = 'none';
    welcomeState.classList.add('hidden');
    
    // Limpiar lista pero mantener el empty state
    const items = container.querySelectorAll('.channel-item');
    items.forEach(item => item.remove());
    
    // Renderizar cada canal
    state.channels.forEach(channel => {
        const item = document.createElement('div');
        item.className = `channel-item ${state.selectedChannel === channel.id ? 'active' : ''}`;
        item.dataset.channelId = channel.id;
        
        item.innerHTML = `
            <span class="status-dot ${channel.status}"></span>
            <div class="channel-info">
                <div class="channel-name">${escapeHtml(channel.label)}</div>
                <div class="channel-srt">${escapeHtml(channel.srtStreamName)}</div>
            </div>
        `;
        
        item.addEventListener('click', () => selectChannel(channel.id));
        container.insertBefore(item, emptyState);
    });
}

function renderChannelGrid() {
    const container = document.getElementById('channelGrid');
    
    if (state.channels.length === 0) {
        container.innerHTML = '';
        return;
    }
    
    container.innerHTML = state.channels.map(channel => `
        <div class="channel-card compact" data-channel-id="${channel.id}">
            <div class="channel-card-header">
                <div class="channel-card-title">
                    <span class="status-indicator ${channel.status}"></span>
                    <h3>${escapeHtml(channel.label)} <span class="channel-id-label">${channel.id.substring(0, 8)}</span></h3>
                </div>
                <div class="channel-card-actions">
                    <button class="btn btn-icon btn-sm" onclick="editChannel('${channel.id}')" title="Editar">
                        <i class="fas fa-edit"></i>
                    </button>
                    <button class="btn btn-icon btn-sm" onclick="confirmDeleteChannel('${channel.id}')" title="Eliminar">
                        <i class="fas fa-trash"></i>
                    </button>
                </div>
            </div>
            <div class="channel-card-body">
                <div class="channel-info-row">
                    <span class="label">SRT</span>
                    <span class="value srt-address">
                        <input type="text" class="inline-input srt-host-input" value="${channel.srtHost || '0.0.0.0'}" 
                            onchange="updateSRTHost('${channel.id}', this.value)" 
                            ${channel.status === 'active' ? 'disabled' : ''} 
                            style="width: 100px;" placeholder="IP">
                        <span>:</span>
                        <span class="srt-port">${channel.srtPort || 9000}</span>
                    </span>
                </div>
                <div class="channel-info-row">
                    <span class="label">Estado</span>
                    <span class="value status-${channel.status}">${getStatusText(channel.status)}</span>
                </div>
                <div class="channel-info-row">
                    <span class="label">Archivo</span>
                    <span class="value file-name" title="${escapeHtml(channel.currentFile || 'Ninguno')}">
                        ${channel.currentFile ? escapeHtml(getFileName(channel.currentFile)) : '<em>Sin archivo</em>'}
                    </span>
                </div>
            </div>
            <div class="channel-card-footer">
                ${channel.status === 'active' 
                    ? `<button class="btn btn-danger btn-sm" onclick="stopChannel('${channel.id}')" title="Detener">
                        <i class="fas fa-stop"></i> Detener
                       </button>`
                    : `<button class="btn btn-warning btn-sm" onclick="playTestPattern('${channel.id}')" title="Iniciar patrón">
                        <i class="fas fa-play"></i> Patrón
                       </button>`
                }
                <button class="btn btn-outline btn-sm" onclick="copySRTUrl('${channel.srtPort || 9000}')" title="Copiar URL SRT">
                    <i class="fas fa-link"></i> URL
                </button>
            </div>
        </div>
    `).join('');
}

// Actualizar solo el estado de una tarjeta sin reconstruir todo el grid
function updateChannelCardStatus(channelId, channel) {
    console.log('[UPDATE] updateChannelCardStatus', channelId, 'status:', channel.status);
    const card = document.querySelector(`[data-channel-id="${channelId}"]`);
    if (!card) {
        console.log('[UPDATE] Card not found for', channelId);
        return;
    }
    
    // Actualizar indicador de estado (LED)
    const indicator = card.querySelector('.status-indicator');
    if (indicator) {
        indicator.className = `status-indicator ${channel.status}`;
        console.log('[UPDATE] LED updated to', channel.status);
    }
    
    // Actualizar texto de estado (buscar por clase que empiece con status-)
    const statusText = card.querySelector('[class*="status-active"], [class*="status-inactive"], [class*="status-error"]');
    if (statusText) {
        statusText.className = `value status-${channel.status}`;
        statusText.textContent = getStatusText(channel.status);
    }
    
    // Actualizar archivo actual
    const fileNameEl = card.querySelector('.file-name');
    if (fileNameEl) {
        fileNameEl.title = channel.currentFile || 'Ninguno';
        fileNameEl.innerHTML = channel.currentFile 
            ? escapeHtml(getFileName(channel.currentFile)) 
            : '<em>Sin archivo</em>';
    }
    
    // Actualizar botón Patrón/Detener
    const footer = card.querySelector('.channel-card-footer');
    if (footer) {
        const firstBtn = footer.querySelector('button:first-child');
        if (firstBtn) {
            console.log('[UPDATE] Updating button, status is:', channel.status);
            if (channel.status === 'active') {
                firstBtn.className = 'btn btn-danger btn-sm';
                firstBtn.title = 'Detener';
                firstBtn.onclick = () => stopChannel(channelId);
                firstBtn.innerHTML = '<i class="fas fa-stop"></i> Detener';
            } else {
                firstBtn.className = 'btn btn-warning btn-sm';
                firstBtn.title = 'Iniciar patrón';
                firstBtn.onclick = () => playTestPattern(channelId);
                firstBtn.innerHTML = '<i class="fas fa-play"></i> Patrón';
            }
        }
    }
}

function renderLogs() {
    const container = document.getElementById('logsContent');
    const filteredLogs = state.logFilter === 'all' 
        ? state.logs 
        : state.logs.filter(log => log.level === state.logFilter);
    
    container.innerHTML = filteredLogs.map(log => `
        <div class="log-entry ${log.level}">
            <span class="timestamp">${log.timestamp}</span>
            <span class="level">[${log.level}]</span>
            <span class="message">${escapeHtml(log.message)}</span>
        </div>
    `).join('');
    
    // Scroll al final
    container.scrollTop = container.scrollHeight;
}

function appendLogEntry(entry) {
    if (state.logFilter !== 'all' && entry.level !== state.logFilter) {
        return;
    }
    
    const container = document.getElementById('logsContent');
    const div = document.createElement('div');
    div.className = `log-entry ${entry.level}`;
    div.innerHTML = `
        <span class="timestamp">${entry.timestamp}</span>
        <span class="level">[${entry.level}]</span>
        <span class="message">${escapeHtml(entry.message)}</span>
    `;
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

// ==================== Acciones de Canal ====================
function selectChannel(channelId) {
    state.selectedChannel = channelId;
    renderChannelList();
}

async function startChannel(channelId) {
    try {
        await window.go.app.App.StartChannel(channelId);
        showToast('success', 'Canal iniciado', 'El stream SRT ha comenzado');
    } catch (error) {
        console.error('Error iniciando canal:', error);
        showToast('error', 'Error', error.message || 'No se pudo iniciar el canal');
    }
}

async function playTestPattern(channelId) {
    try {
        await window.go.app.App.PlayTestPattern(channelId);
        showToast('success', 'Patrón de prueba', 'Emitiendo patrón de prueba en el canal');
    } catch (error) {
        console.error('Error emitiendo patrón:', error);
        showToast('error', 'Error', error.message || 'No se pudo emitir el patrón. ¿Está configurada la ruta?');
    }
}

async function updateSRTHost(channelId, host) {
    try {
        await window.go.app.App.SetChannelSRTHost(channelId, host);
        
        // Actualizar estado local
        const index = state.channels.findIndex(c => c.id === channelId);
        if (index !== -1) {
            state.channels[index].srtHost = host;
        }
        
        showToast('success', 'IP SRT actualizada', `Stream en: ${host}`);
    } catch (error) {
        console.error('Error actualizando SRT host:', error);
        showToast('error', 'Error', error.message || 'No se pudo actualizar la IP');
    }
}

function copySRTUrl(port) {
    // Intentar obtener la IP del servidor desde la configuración o usar placeholder
    const serverIP = state.serverIP || 'IP_SERVIDOR';
    const srtUrl = `srt://${serverIP}:${port}`;
    
    navigator.clipboard.writeText(srtUrl).then(() => {
        showToast('success', 'URL copiada', `${srtUrl} copiado al portapapeles`);
    }).catch(() => {
        // Fallback
        prompt('URL SRT:', srtUrl);
    });
}

async function stopChannel(channelId) {
    try {
        await window.go.app.App.StopChannel(channelId);
        showToast('info', 'Canal detenido', 'El stream SRT ha sido detenido');
    } catch (error) {
        console.error('Error deteniendo canal:', error);
        showToast('error', 'Error', error.message || 'No se pudo detener el canal');
    }
}

async function stopAllStreams() {
    try {
        await window.go.app.App.StopAllStreams();
        showToast('warning', 'Streams detenidos', 'Todos los streams han sido detenidos de forma forzada');
    } catch (error) {
        console.error('Error deteniendo todos los streams:', error);
        showToast('error', 'Error', error.message || 'No se pudieron detener los streams');
    }
}

async function deleteChannel(channelId) {
    try {
        await window.go.app.App.RemoveChannel(channelId);
        showToast('success', 'Canal eliminado', 'El canal ha sido eliminado correctamente');
        closeConfirmModal();
    } catch (error) {
        console.error('Error eliminando canal:', error);
        showToast('error', 'Error', error.message || 'No se pudo eliminar el canal');
    }
}

function editChannel(channelId) {
    const channel = state.channels.find(c => c.id === channelId);
    if (!channel) return;
    
    document.getElementById('channelModalTitle').textContent = 'Editar Canal';
    document.getElementById('channelId').value = channel.id;
    document.getElementById('channelLabel').value = channel.label;
    document.getElementById('channelSRTName').value = channel.srtStreamName;
    
    openModal('channelModal');
}

function confirmDeleteChannel(channelId) {
    const channel = state.channels.find(c => c.id === channelId);
    if (!channel) return;
    
    document.getElementById('confirmTitle').textContent = 'Eliminar Canal';
    document.getElementById('confirmMessage').textContent = 
        `¿Está seguro de eliminar el canal "${channel.label}"? Esta acción no se puede deshacer.`;
    
    const btnConfirm = document.getElementById('btnConfirmOk');
    btnConfirm.onclick = () => deleteChannel(channelId);
    
    openModal('confirmModal');
}

// ==================== Modales ====================
function openModal(modalId) {
    document.getElementById(modalId)?.classList.add('open');
}

function closeModal(modalId) {
    document.getElementById(modalId)?.classList.remove('open');
}

function closeAllModals() {
    document.querySelectorAll('.modal.open').forEach(modal => {
        modal.classList.remove('open');
    });
}

function openChannelModal() {
    document.getElementById('channelModalTitle').textContent = 'Nuevo Canal';
    document.getElementById('channelForm').reset();
    document.getElementById('channelId').value = '';
    openModal('channelModal');
}

function closeChannelModal() {
    closeModal('channelModal');
}

async function saveChannel() {
    const id = document.getElementById('channelId').value;
    const label = document.getElementById('channelLabel').value.trim();
    const srtStreamName = document.getElementById('channelSRTName').value.trim();
    
    if (!label) {
        showToast('warning', 'Campo requerido', 'Ingrese un nombre para el canal');
        return;
    }
    
    try {
        if (id) {
            // Actualizar canal existente
            await window.go.app.App.UpdateChannel(id, label, srtStreamName);
            showToast('success', 'Canal actualizado', `${label} ha sido actualizado`);
        } else {
            // Crear nuevo canal
            await window.go.app.App.AddChannel(label, srtStreamName);
            showToast('success', 'Canal creado', `${label} ha sido agregado`);
        }
        closeChannelModal();
    } catch (error) {
        console.error('Error guardando canal:', error);
        showToast('error', 'Error', error.message || 'No se pudo guardar el canal');
    }
}

function openSettingsModal() {
    if (state.config) {
        applyConfigToForm();
    }
    openModal('settingsModal');
}

function closeSettingsModal() {
    closeModal('settingsModal');
}

function switchSettingsTab(tabId) {
    document.querySelectorAll('.settings-tabs .tab-btn').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.tab === tabId);
    });
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.toggle('active', content.id === `tab-${tabId}`);
    });
}

async function saveSettings() {
    try {
        const config = getConfigFromForm();
        
        await window.go.app.App.UpdateConfig(config);
        state.config = config;
        applyConfig();
        closeSettingsModal();
        showToast('success', 'Configuración guardada', 'Los cambios han sido aplicados');
    } catch (error) {
        console.error('Error guardando configuración:', error);
        showToast('error', 'Error', 'No se pudo guardar la configuración');
    }
}

function applyConfigToForm() {
    if (!state.config) return;
    
    // General
    document.getElementById('settingsWSPort').value = state.config.webSocketPort || 8765;
    document.getElementById('settingsFFmpegPath').value = state.config.ffmpegPath || 'ffmpeg';
    document.getElementById('settingsAutoRestart').checked = state.config.autoRestart !== false;
    document.getElementById('settingsTestPattern').value = state.config.testPatternPath || '';
    document.getElementById('settingsSRTPrefix').value = state.config.srtPrefix || 'SRT_SERVER_';
    document.getElementById('settingsTheme').value = state.config.theme || 'dark';
    
    // Encoding
    document.getElementById('settingsVideoEncoder').value = state.config.videoEncoder || 'libx264';
    document.getElementById('settingsEncoderPreset').value = state.config.encoderPreset || 'veryfast';
    document.getElementById('settingsEncoderProfile').value = state.config.encoderProfile || 'main';
    document.getElementById('settingsEncoderTune').value = state.config.encoderTune || 'zerolatency';
    document.getElementById('settingsGopSize').value = state.config.gopSize || 50;
    document.getElementById('settingsBFrames').value = state.config.bFrames || 0;
    
    // Bitrate
    document.getElementById('settingsBitrateMode').value = state.config.bitrateMode || 'cbr';
    document.getElementById('settingsVideoBitrate').value = state.config.defaultVideoBitrate || '5M';
    document.getElementById('settingsAudioBitrate').value = state.config.defaultAudioBitrate || '192k';
    document.getElementById('settingsMaxBitrate').value = state.config.maxBitrate || '5M';
    document.getElementById('settingsBufferSize').value = state.config.bufferSize || '5M';
    document.getElementById('settingsFrameRate').value = state.config.defaultFrameRate || 25;
    
    // SRT
    document.getElementById('settingsSRTLatency').value = state.config.srtLatency || 500;
    document.getElementById('settingsSRTRecvBuffer').value = (state.config.srtRecvBuffer || 8388608) / 1048576; // Convertir a MB
    document.getElementById('settingsSRTSendBuffer').value = (state.config.srtSendBuffer || 8388608) / 1048576;
    document.getElementById('settingsSRTOverheadBW').value = state.config.srtOverheadBW || 25;
    document.getElementById('settingsSRTPeerIdleTime').value = state.config.srtPeerIdleTime || 5000;
}

function getConfigFromForm() {
    return {
        // General
        webSocketPort: parseInt(document.getElementById('settingsWSPort').value) || 8765,
        ffmpegPath: document.getElementById('settingsFFmpegPath').value || 'ffmpeg',
        autoRestart: document.getElementById('settingsAutoRestart').checked,
        testPatternPath: document.getElementById('settingsTestPattern').value || '',
        srtPrefix: document.getElementById('settingsSRTPrefix').value || 'SRT_SERVER_',
        theme: document.getElementById('settingsTheme').value || 'dark',
        language: state.config?.language || 'es',
        maxLogLines: state.config?.maxLogLines || 1000,
        defaultVideoPath: state.config?.defaultVideoPath || '',
        logPath: state.config?.logPath || '',
        srtGroup: state.config?.srtGroup || '',
        
        // Encoding
        videoEncoder: document.getElementById('settingsVideoEncoder').value || 'libx264',
        encoderPreset: document.getElementById('settingsEncoderPreset').value || 'veryfast',
        encoderProfile: document.getElementById('settingsEncoderProfile').value || 'main',
        encoderTune: document.getElementById('settingsEncoderTune').value || 'zerolatency',
        gopSize: parseInt(document.getElementById('settingsGopSize').value) || 50,
        bFrames: parseInt(document.getElementById('settingsBFrames').value) || 0,
        
        // Bitrate
        bitrateMode: document.getElementById('settingsBitrateMode').value || 'cbr',
        defaultVideoBitrate: document.getElementById('settingsVideoBitrate').value || '5M',
        defaultAudioBitrate: document.getElementById('settingsAudioBitrate').value || '192k',
        maxBitrate: document.getElementById('settingsMaxBitrate').value || '5M',
        bufferSize: document.getElementById('settingsBufferSize').value || '5M',
        defaultFrameRate: parseInt(document.getElementById('settingsFrameRate').value) || 25,
        crf: state.config?.crf || 23,
        
        // SRT (convertir MB a bytes)
        srtLatency: parseInt(document.getElementById('settingsSRTLatency').value) || 500,
        srtRecvBuffer: (parseInt(document.getElementById('settingsSRTRecvBuffer').value) || 8) * 1048576,
        srtSendBuffer: (parseInt(document.getElementById('settingsSRTSendBuffer').value) || 8) * 1048576,
        srtOverheadBW: parseInt(document.getElementById('settingsSRTOverheadBW').value) || 25,
        srtPeerIdleTime: parseInt(document.getElementById('settingsSRTPeerIdleTime').value) || 5000
    };
}

function closeConfirmModal() {
    closeModal('confirmModal');
}

// ==================== Logs Panel ====================
function toggleLogsPanel() {
    const panel = document.getElementById('logsPanel');
    panel.classList.toggle('open');
    
    if (panel.classList.contains('open') && state.logs.length === 0) {
        loadLogs();
    }
}

async function clearLogs() {
    try {
        await window.go.app.App.ClearLogs();
        state.logs = [];
        document.getElementById('logsContent').innerHTML = '';
        showToast('info', 'Logs limpiados', 'El historial de logs ha sido borrado');
    } catch (error) {
        console.error('Error limpiando logs:', error);
    }
}

// ==================== UI Helpers ====================
function updateChannelCount() {
    const countEl = document.querySelector('#channelCount strong');
    if (countEl) {
        countEl.textContent = state.channels.length;
    }
}

function updateClientCount() {
    const countEl = document.querySelector('#clientCount strong');
    if (countEl) {
        countEl.textContent = state.connectedClients.length;
    }
}

function applyConfig() {
    if (state.config?.theme === 'light') {
        document.body.classList.add('light-theme');
    } else {
        document.body.classList.remove('light-theme');
    }
}

function showToast(type, title, message) {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    
    const icons = {
        success: 'fa-check-circle',
        error: 'fa-exclamation-circle',
        warning: 'fa-exclamation-triangle',
        info: 'fa-info-circle'
    };
    
    toast.innerHTML = `
        <i class="fas ${icons[type] || icons.info}"></i>
        <div class="toast-content">
            <div class="toast-title">${escapeHtml(title)}</div>
            <div class="toast-message">${escapeHtml(message)}</div>
        </div>
        <button class="toast-close" onclick="this.parentElement.remove()">
            <i class="fas fa-times"></i>
        </button>
    `;
    
    container.appendChild(toast);
    
    // Auto-remove después de 5 segundos
    setTimeout(() => {
        toast.style.animation = 'slideIn 0.3s ease reverse';
        setTimeout(() => toast.remove(), 300);
    }, 5000);
}

// ==================== Utilidades ====================
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function getFileName(path) {
    if (!path) return 'Sin archivo';
    return path.split(/[\\/]/).pop();
}

function getStatusText(status) {
    const statusMap = {
        'active': 'Activo',
        'inactive': 'Inactivo',
        'error': 'Error',
        'starting': 'Iniciando',
        'stopping': 'Deteniendo'
    };
    return statusMap[status] || status;
}

// Modo desarrollo sin Wails
function showMockData() {
    console.log('Mostrando datos de prueba...');
    state.channels = [
        {
            id: 'test-1',
            label: 'Canal Principal',
            videoPath: 'C:\\Videos\\test.mp4',
            srtStreamName: 'SRT_CANAL_1',
            status: 'inactive',
            currentFile: 'C:\\Videos\\test.mp4'
        },
        {
            id: 'test-2',
            label: 'Canal Secundario',
            videoPath: 'C:\\Videos\\demo.mp4',
            srtStreamName: 'SRT_CANAL_2',
            status: 'active',
            currentFile: 'C:\\Videos\\demo.mp4'
        }
    ];
    
    renderChannelList();
    renderChannelGrid();
    updateChannelCount();
}

// Exponer funciones globales para onclick
window.startChannel = startChannel;
window.stopChannel = stopChannel;
window.editChannel = editChannel;
window.confirmDeleteChannel = confirmDeleteChannel;
window.playTestPattern = playTestPattern;
window.copySRTUrl = copySRTUrl;
window.updateSRTHost = updateSRTHost;
