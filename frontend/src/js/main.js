/**
 * NDI Server Stream - JavaScript Principal
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
    console.log('NDI Server Stream - Inicializando...');
    
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
        const existing = state.channels.findIndex(c => c.id === channel.id);
        if (existing === -1) {
            state.channels.push(channel);
        } else {
            state.channels[existing] = channel;
        }
        renderChannelList();
        renderChannelGrid();
        updateChannelCount();
    });
    
    // Canal eliminado
    window.runtime.EventsOn('channel:removed', (channelId) => {
        state.channels = state.channels.filter(c => c.id !== channelId);
        renderChannelList();
        renderChannelGrid();
        updateChannelCount();
    });
    
    // Canal actualizado
    window.runtime.EventsOn('channel:updated', (channel) => {
        const index = state.channels.findIndex(c => c.id === channel.id);
        if (index !== -1) {
            state.channels[index] = channel;
            renderChannelList();
            renderChannelGrid();
        }
    });
    
    // Estado del canal
    window.runtime.EventsOn('channel:status', (data) => {
        const index = state.channels.findIndex(c => c.id === data.channelId);
        if (index !== -1) {
            state.channels[index].status = data.status;
            if (data.currentFile) {
                state.channels[index].currentFile = data.currentFile;
            }
            renderChannelList();
            renderChannelGrid();
        }
    });
    
    // Previsualización del canal
    window.runtime.EventsOn('channel:preview', (data) => {
        const card = document.querySelector(`[data-channel-id="${data.channelId}"] .preview-image`);
        if (card && data.preview) {
            card.src = data.preview;
            card.style.display = 'block';
            card.parentElement.querySelector('.no-preview')?.classList.add('hidden');
        }
    });
    
    // Progreso del canal
    window.runtime.EventsOn('channel:progress', (data) => {
        const statsEl = document.querySelector(`[data-channel-id="${data.channelId}"] .preview-stats`);
        if (statsEl && data.progress) {
            statsEl.innerHTML = `
                <span><i class="fas fa-film"></i> ${data.progress.frame || 0}</span>
                <span><i class="fas fa-tachometer-alt"></i> ${data.progress.fps?.toFixed(1) || 0} fps</span>
                <span><i class="fas fa-clock"></i> ${data.progress.time || '00:00:00'}</span>
            `;
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
}

// ==================== Event Listeners de UI ====================
function setupEventListeners() {
    // Botones principales
    document.getElementById('btnAddChannel')?.addEventListener('click', () => openChannelModal());
    document.getElementById('btnAddChannelEmpty')?.addEventListener('click', () => openChannelModal());
    document.getElementById('btnGetStarted')?.addEventListener('click', () => openChannelModal());
    document.getElementById('btnSettings')?.addEventListener('click', () => openSettingsModal());
    document.getElementById('btnToggleLogs')?.addEventListener('click', toggleLogsPanel);
    
    // Modal de canal
    document.getElementById('btnCloseChannelModal')?.addEventListener('click', closeChannelModal);
    document.getElementById('btnCancelChannel')?.addEventListener('click', closeChannelModal);
    document.getElementById('btnSaveChannel')?.addEventListener('click', saveChannel);
    document.getElementById('btnSelectVideo')?.addEventListener('click', selectVideoPath);
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
    
    // Preview quality slider
    document.getElementById('settingsPreviewQuality')?.addEventListener('input', (e) => {
        document.getElementById('previewQualityValue').textContent = e.target.value + '%';
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
                <div class="channel-ndi">${escapeHtml(channel.ndiStreamName)}</div>
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
        <div class="channel-card" data-channel-id="${channel.id}">
            <div class="channel-card-header">
                <div class="channel-card-title">
                    <span class="status-indicator ${channel.status}"></span>
                    <h3>${escapeHtml(channel.label)}</h3>
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
            <div class="channel-card-preview">
                ${channel.previewBase64 
                    ? `<img src="${channel.previewBase64}" class="preview-image" alt="Preview">`
                    : `<div class="no-preview">
                        <i class="fas fa-satellite-dish"></i>
                        <span>Esperando solicitud de Aximmetry...</span>
                       </div>`
                }
                <div class="preview-overlay">
                    <div class="preview-stats">
                        <span><i class="fas fa-film"></i> 0</span>
                        <span><i class="fas fa-tachometer-alt"></i> 0 fps</span>
                    </div>
                </div>
            </div>
            <div class="channel-card-body">
                <div class="channel-info-row">
                    <span class="label">SRT Puerto</span>
                    <span class="value srt-port">${channel.srtPort || 9000}</span>
                </div>
                <div class="channel-info-row">
                    <span class="label">Estado</span>
                    <span class="value status-${channel.status}">${getStatusText(channel.status)}</span>
                </div>
                <div class="channel-info-row">
                    <span class="label">Reproduciendo</span>
                    <span class="value" title="${escapeHtml(channel.currentFile || 'Ninguno')}">
                        ${channel.currentFile ? escapeHtml(getFileName(channel.currentFile)) : '<em>Esperando video...</em>'}
                    </span>
                </div>
            </div>
            <div class="channel-card-footer">
                <button class="btn btn-warning btn-sm" onclick="playTestPattern('${channel.id}')" title="Emitir patrón de prueba">
                    <i class="fas fa-broadcast-tower"></i> Patrón
                </button>
                ${channel.status === 'active' 
                    ? `<button class="btn btn-danger btn-sm" onclick="stopChannel('${channel.id}')">
                        <i class="fas fa-stop"></i> Detener
                       </button>`
                    : `<button class="btn btn-outline btn-sm" disabled title="Aximmetry inicia la reproducción">
                        <i class="fas fa-satellite-dish"></i> Listo
                       </button>`
                }
                <button class="btn btn-outline btn-sm" onclick="copySRTUrl('${channel.srtPort || 9000}')" title="Copiar URL SRT">
                    <i class="fas fa-copy"></i> SRT
                </button>
            </div>
        </div>
    `).join('');
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
        showToast('success', 'Canal iniciado', 'El stream NDI ha comenzado');
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
    document.getElementById('channelVideoPath').value = channel.videoPath;
    document.getElementById('channelNDIName').value = channel.ndiStreamName;
    document.getElementById('channelPreviewEnabled').checked = channel.previewEnabled;
    
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

async function selectVideoFile(channelId) {
    try {
        const path = await window.go.app.App.SelectVideoPath();
        if (path) {
            await window.go.app.App.PlayVideoOnChannel(channelId, path);
            showToast('success', 'Video seleccionado', `Reproduciendo: ${getFileName(path)}`);
        }
    } catch (error) {
        console.error('Error seleccionando video:', error);
    }
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
    const ndiStreamName = document.getElementById('channelNDIName').value.trim();
    
    if (!label) {
        showToast('warning', 'Campo requerido', 'Ingrese un nombre para el canal');
        return;
    }
    
    try {
        if (id) {
            // Actualizar canal existente
            await window.go.app.App.UpdateChannel(id, label, ndiStreamName);
            showToast('success', 'Canal actualizado', `${label} ha sido actualizado`);
        } else {
            // Crear nuevo canal
            await window.go.app.App.AddChannel(label, ndiStreamName);
            showToast('success', 'Canal creado', `${label} ha sido agregado`);
        }
        closeChannelModal();
    } catch (error) {
        console.error('Error guardando canal:', error);
        showToast('error', 'Error', error.message || 'No se pudo guardar el canal');
    }
}

// selectVideoPath ya no es necesario - Aximmetry envía el video
async function selectVideoPath() {
    showToast('info', 'Información', 'Aximmetry envía la ruta del video vía WebSocket');
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
        const config = {
            webSocketPort: parseInt(document.getElementById('settingsWSPort').value),
            ffmpegPath: document.getElementById('settingsFFmpegPath').value,
            autoRestart: document.getElementById('settingsAutoRestart').checked,
            defaultVideoBitrate: document.getElementById('settingsVideoBitrate').value,
            defaultAudioBitrate: document.getElementById('settingsAudioBitrate').value,
            defaultFrameRate: parseInt(document.getElementById('settingsFrameRate').value),
            testPatternPath: document.getElementById('settingsTestPattern').value,
            previewConfig: {
                width: parseInt(document.getElementById('settingsPreviewWidth').value),
                height: parseInt(document.getElementById('settingsPreviewHeight').value),
                quality: parseInt(document.getElementById('settingsPreviewQuality').value),
                updateIntervalMs: parseInt(document.getElementById('settingsPreviewInterval').value),
                enabled: true
            },
            ndiPrefix: document.getElementById('settingsNDIPrefix').value,
            theme: document.getElementById('settingsTheme').value
        };
        
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
    
    document.getElementById('settingsWSPort').value = state.config.webSocketPort || 8765;
    document.getElementById('settingsFFmpegPath').value = state.config.ffmpegPath || 'ffmpeg';
    document.getElementById('settingsAutoRestart').checked = state.config.autoRestart !== false;
    document.getElementById('settingsVideoBitrate').value = state.config.defaultVideoBitrate || '10M';
    document.getElementById('settingsAudioBitrate').value = state.config.defaultAudioBitrate || '192k';
    document.getElementById('settingsFrameRate').value = state.config.defaultFrameRate || 30;
    document.getElementById('settingsTestPattern').value = state.config.testPatternPath || '';
    document.getElementById('settingsNDIPrefix').value = state.config.ndiPrefix || 'NDI_SERVER_';
    document.getElementById('settingsTheme').value = state.config.theme || 'dark';
    
    if (state.config.previewConfig) {
        document.getElementById('settingsPreviewWidth').value = state.config.previewConfig.width || 320;
        document.getElementById('settingsPreviewHeight').value = state.config.previewConfig.height || 180;
        document.getElementById('settingsPreviewQuality').value = state.config.previewConfig.quality || 60;
        document.getElementById('settingsPreviewInterval').value = state.config.previewConfig.updateIntervalMs || 2000;
        document.getElementById('previewQualityValue').textContent = (state.config.previewConfig.quality || 60) + '%';
    }
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
            ndiStreamName: 'NDI_CANAL_1',
            status: 'inactive',
            previewEnabled: true,
            currentFile: 'C:\\Videos\\test.mp4'
        },
        {
            id: 'test-2',
            label: 'Canal Secundario',
            videoPath: 'C:\\Videos\\demo.mp4',
            ndiStreamName: 'NDI_CANAL_2',
            status: 'active',
            previewEnabled: true,
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
window.selectVideoFile = selectVideoFile;
