package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"servidor-stream/internal/channel"
	"servidor-stream/internal/config"
	"servidor-stream/internal/ffmpeg"
	"servidor-stream/internal/websocket"
)

// App estructura principal de la aplicación
type App struct {
	ctx            context.Context
	channelManager *channel.Manager
	wsServer       *websocket.Server
	ffmpegManager  *ffmpeg.Manager
	config         *config.Config
	logBuffer      []LogEntry
	logMutex       sync.RWMutex
	cancelFunc     context.CancelFunc
}

// LogEntry representa una entrada de log
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	ChannelID string `json:"channelId,omitempty"`
}

// NewApp crea una nueva instancia de la aplicación
func NewApp() *App {
	return &App{
		logBuffer: make([]LogEntry, 0, 1000),
	}
}

// Startup es llamado cuando la aplicación inicia
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	cancelCtx, cancel := context.WithCancel(ctx)
	a.cancelFunc = cancel

	// Cargar configuración
	cfg, err := config.Load()
	if err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Error cargando configuración: %v", err), "")
		cfg = config.Default()
	}
	a.config = cfg

	// Inicializar managers
	a.channelManager = channel.NewManager()
	a.ffmpegManager = ffmpeg.NewManager(cfg.FFmpegPath, a.onFFmpegEvent)

	// Inicializar servidor WebSocket
	a.wsServer = websocket.NewServer(cfg.WebSocketPort, a.handleWebSocketMessage)

	// Configurar callbacks para eventos de clientes
	a.wsServer.SetClientCallbacks(
		func(client websocket.ClientInfo) {
			a.AddLog("INFO", fmt.Sprintf("Cliente conectado: %s (%s)", client.Name, client.RemoteAddr), "")
			runtime.EventsEmit(a.ctx, "client:connected", client)
		},
		func(clientID string) {
			a.AddLog("INFO", fmt.Sprintf("Cliente desconectado: %s", clientID), "")
			runtime.EventsEmit(a.ctx, "client:disconnected", clientID)
		},
	)

	go a.wsServer.Start(cancelCtx)

	// Iniciar monitor de canales
	go a.monitorChannels(cancelCtx)

	a.AddLog("INFO", fmt.Sprintf("SRT Server Stream iniciado en puerto WebSocket %d", cfg.WebSocketPort), "")
}

// Shutdown es llamado cuando la aplicación se cierra
func (a *App) Shutdown(ctx context.Context) {
	a.AddLog("INFO", "Cerrando SRT Server Stream...", "")

	// Cancelar contexto
	if a.cancelFunc != nil {
		a.cancelFunc()
	}

	// Detener todos los streams
	if a.ffmpegManager != nil {
		a.ffmpegManager.StopAll()
	}

	// Detener servidor WebSocket
	if a.wsServer != nil {
		a.wsServer.Stop()
	}

	// Guardar configuración
	if a.config != nil {
		config.Save(a.config)
	}

	a.AddLog("INFO", "SRT Server Stream cerrado correctamente", "")
}

// DomReady es llamado cuando el DOM está listo
func (a *App) DomReady(ctx context.Context) {
	// Cargar canales guardados
	channels := a.channelManager.GetAll()
	for _, ch := range channels {
		runtime.EventsEmit(a.ctx, "channel:added", ch)
	}
}

// ==================== API para Frontend ====================

// GetChannels retorna todos los canales
func (a *App) GetChannels() []channel.Channel {
	return a.channelManager.GetAll()
}

// AddChannel agrega un nuevo canal (sin videoPath - Aximmetry lo envía vía WebSocket)
func (a *App) AddChannel(label, srtStreamName string) (*channel.Channel, error) {
	ch, err := a.channelManager.Add(label, "", srtStreamName) // videoPath vacío inicialmente
	if err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Error agregando canal: %v", err), "")
		return nil, err
	}

	a.AddLog("INFO", fmt.Sprintf("Canal agregado: %s (%s)", ch.Label, ch.ID), ch.ID)
	runtime.EventsEmit(a.ctx, "channel:added", ch)

	return ch, nil
}

// RemoveChannel elimina un canal
func (a *App) RemoveChannel(channelID string) error {
	// Detener stream si está activo
	a.ffmpegManager.Stop(channelID)

	// Eliminar canal
	err := a.channelManager.Remove(channelID)
	if err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Error eliminando canal %s: %v", channelID, err), channelID)
		return err
	}

	a.AddLog("INFO", fmt.Sprintf("Canal eliminado: %s", channelID), channelID)
	runtime.EventsEmit(a.ctx, "channel:removed", channelID)

	return nil
}

// UpdateChannel actualiza la configuración de un canal (sin videoPath)
func (a *App) UpdateChannel(channelID, label, srtStreamName string) (*channel.Channel, error) {
	ch, err := a.channelManager.Update(channelID, label, "", srtStreamName) // videoPath se mantiene
	if err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Error actualizando canal %s: %v", channelID, err), channelID)
		return nil, err
	}

	a.AddLog("INFO", fmt.Sprintf("Canal actualizado: %s", label), channelID)
	runtime.EventsEmit(a.ctx, "channel:updated", ch)

	return ch, nil
}

// StartChannel inicia el stream de un canal
func (a *App) StartChannel(channelID string) error {
	ch, err := a.channelManager.Get(channelID)
	if err != nil {
		return err
	}

	// Usar CurrentFile si VideoPath está vacío (ej: si estaba reproduciendo patrón)
	inputPath := ch.VideoPath
	if inputPath == "" && ch.CurrentFile != "" {
		inputPath = ch.CurrentFile
	}

	// Parsear resolución del canal
	width, height := 1920, 1080
	if ch.Resolution != "" {
		fmt.Sscanf(ch.Resolution, "%dx%d", &width, &height)
	}

	// Usar FPS del canal o el por defecto
	frameRate := ch.FrameRate
	if frameRate == 0 {
		frameRate = a.config.DefaultFrameRate
	}

	// Configurar y iniciar FFmpeg con SRT
	ffmpegConfig := ffmpeg.StreamConfig{
		ChannelID:     ch.ID,
		InputPath:     inputPath,
		SRTStreamName: ch.SRTStreamName,
		SRTPort:       ch.SRTPort,
		SRTHost:       ch.SRTHost,
		VideoBitrate:  a.config.DefaultVideoBitrate,
		AudioBitrate:  a.config.DefaultAudioBitrate,
		FrameRate:     frameRate,
		Width:         width,
		Height:        height,
		Loop:          true, // Loop por defecto
		// Configuración avanzada
		VideoEncoder:   a.config.VideoEncoder,
		EncoderPreset:  a.config.EncoderPreset,
		EncoderProfile: a.config.EncoderProfile,
		EncoderTune:    a.config.EncoderTune,
		GopSize:        a.config.GopSize,
		BFrames:        a.config.BFrames,
		BitrateMode:    a.config.BitrateMode,
		MaxBitrate:     a.config.MaxBitrate,
		BufferSize:     a.config.BufferSize,
		SRTLatency:     a.config.SRTLatency,
		SRTRecvBuffer:  a.config.SRTRecvBuffer,
		SRTSendBuffer:  a.config.SRTSendBuffer,
		SRTOverheadBW:  a.config.SRTOverheadBW,
	}

	err = a.ffmpegManager.StartWithFallback(ffmpegConfig)
	if err != nil {
		a.channelManager.SetStatus(channelID, channel.StatusError)
		a.AddLog("ERROR", fmt.Sprintf("Error iniciando stream %s: %v", ch.Label, err), channelID)
		return err
	}

	a.channelManager.SetStatus(channelID, channel.StatusActive)
	a.AddLog("INFO", fmt.Sprintf("Stream SRT iniciado: %s -> srt://%s:%d", ch.Label, ch.SRTHost, ch.SRTPort), channelID)
	runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
		"channelId": channelID,
		"status":    channel.StatusActive,
		"srtPort":   ch.SRTPort,
	})

	return nil
}

// StopAllStreams detiene todos los streams FFmpeg de forma forzada sin reinicio
func (a *App) StopAllStreams() error {
	a.AddLog("INFO", "Deteniendo todos los streams de forma forzada...", "")

	// Obtener todos los canales
	channels := a.channelManager.GetAll()

	// Detener cada proceso FFmpeg
	a.ffmpegManager.StopAll()

	// Actualizar el estado de todos los canales a inactivo
	for _, ch := range channels {
		a.channelManager.SetStatus(ch.ID, channel.StatusInactive)
		runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
			"channelId": ch.ID,
			"status":    channel.StatusInactive,
			"event":     "force_stopped",
		})
	}

	a.AddLog("INFO", fmt.Sprintf("Se detuvieron %d streams de forma forzada", len(channels)), "")

	return nil
}

// PlayTestPattern reproduce el patrón de prueba en un canal
func (a *App) PlayTestPattern(channelID string) error {
	a.AddLog("INFO", fmt.Sprintf("PlayTestPattern llamado para canal: %s", channelID), channelID)

	// Verificar que el patrón está configurado
	if a.config.TestPatternPath == "" {
		a.AddLog("ERROR", "Patrón de prueba no configurado", channelID)
		return fmt.Errorf("patrón de prueba no configurado. Configure la ruta en Ajustes")
	}

	a.AddLog("INFO", fmt.Sprintf("Patrón configurado: %s", a.config.TestPatternPath), channelID)

	// Verificar que el archivo existe
	if _, err := os.Stat(a.config.TestPatternPath); os.IsNotExist(err) {
		a.AddLog("ERROR", fmt.Sprintf("Archivo no encontrado: %s", a.config.TestPatternPath), channelID)
		return fmt.Errorf("archivo de patrón no encontrado: %s", a.config.TestPatternPath)
	}

	ch, err := a.channelManager.Get(channelID)
	if err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Canal no encontrado: %v", err), channelID)
		return err
	}

	a.AddLog("INFO", fmt.Sprintf("Canal encontrado: %s, puerto SRT: %d", ch.Label, ch.SRTPort), channelID)

	// Si el canal está activo, detenerlo primero
	if ch.Status == channel.StatusActive {
		a.ffmpegManager.Stop(channelID)
	}

	// Actualizar el archivo actual a patrón
	a.channelManager.SetCurrentFile(channelID, a.config.TestPatternPath)

	// Parsear resolución del canal
	width, height := 1920, 1080 // Valores por defecto
	if ch.Resolution != "" {
		fmt.Sscanf(ch.Resolution, "%dx%d", &width, &height)
	}

	// Usar FPS del canal o el por defecto
	frameRate := ch.FrameRate
	if frameRate == 0 {
		frameRate = a.config.DefaultFrameRate
	}

	// Configurar y iniciar FFmpeg con el patrón
	ffmpegConfig := ffmpeg.StreamConfig{
		ChannelID:     ch.ID,
		InputPath:     a.config.TestPatternPath,
		SRTStreamName: ch.SRTStreamName,
		SRTPort:       ch.SRTPort,
		SRTHost:       ch.SRTHost,
		VideoBitrate:  a.config.DefaultVideoBitrate,
		AudioBitrate:  a.config.DefaultAudioBitrate,
		FrameRate:     frameRate,
		Width:         width,
		Height:        height,
		Loop:          true, // El patrón siempre en loop
		// Configuración avanzada de encoding
		VideoEncoder:   a.config.VideoEncoder,
		EncoderPreset:  a.config.EncoderPreset,
		EncoderProfile: a.config.EncoderProfile,
		EncoderTune:    a.config.EncoderTune,
		GopSize:        a.config.GopSize,
		BFrames:        a.config.BFrames,
		// Control de bitrate
		BitrateMode: a.config.BitrateMode,
		MaxBitrate:  a.config.MaxBitrate,
		BufferSize:  a.config.BufferSize,
		// SRT avanzado
		SRTLatency:    a.config.SRTLatency,
		SRTRecvBuffer: a.config.SRTRecvBuffer,
		SRTSendBuffer: a.config.SRTSendBuffer,
		SRTOverheadBW: a.config.SRTOverheadBW,
	}

	a.AddLog("INFO", fmt.Sprintf("Iniciando FFmpeg: %dx%d @ %dfps en %s:%d (encoder: %s)", width, height, frameRate, ch.SRTHost, ch.SRTPort, a.config.VideoEncoder), channelID)

	err = a.ffmpegManager.StartWithFallback(ffmpegConfig)
	if err != nil {
		a.channelManager.SetStatus(channelID, channel.StatusError)
		a.AddLog("ERROR", fmt.Sprintf("Error iniciando patrón de prueba: %v", err), channelID)
		return err
	}

	a.channelManager.SetStatus(channelID, channel.StatusActive)
	a.AddLog("INFO", fmt.Sprintf("Patrón de prueba iniciado en %s (SRT puerto %d)", ch.Label, ch.SRTPort), channelID)

	// Emitir evento de status ACTIVO al frontend
	a.AddLog("INFO", fmt.Sprintf("Emitiendo channel:status con status=active para canal %s", channelID), channelID)
	runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
		"channelId":     channelID,
		"status":        "active", // Usar string directo para asegurar compatibilidad
		"currentFile":   "[PATRÓN DE PRUEBA]",
		"srtPort":       ch.SRTPort,
		"isTestPattern": true,
	})

	return nil
}

// SelectTestPatternPath abre un diálogo para seleccionar el archivo de patrón
func (a *App) SelectTestPatternPath() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Seleccionar video de patrón",
		Filters: []runtime.FileFilter{
			{DisplayName: "Videos", Pattern: "*.mp4;*.mov;*.avi;*.mkv;*.webm"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// SetChannelSRTHost establece la IP/Host SRT de un canal
func (a *App) SetChannelSRTHost(channelID, host string) error {
	err := a.channelManager.SetSRTHost(channelID, host)
	if err != nil {
		return err
	}

	a.AddLog("INFO", fmt.Sprintf("IP SRT actualizada: %s", host), channelID)
	runtime.EventsEmit(a.ctx, "channel:updated", nil)
	return nil
}

// StopChannel detiene el stream de un canal
func (a *App) StopChannel(channelID string) error {
	ch, err := a.channelManager.Get(channelID)
	if err != nil {
		return err
	}

	err = a.ffmpegManager.Stop(channelID)
	if err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Error deteniendo stream %s: %v", ch.Label, err), channelID)
		return err
	}

	a.channelManager.SetStatus(channelID, channel.StatusInactive)
	a.AddLog("INFO", fmt.Sprintf("Stream detenido: %s", ch.Label), channelID)
	runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
		"channelId": channelID,
		"status":    channel.StatusInactive,
	})

	return nil
}

// ToggleChannel activa o desactiva un canal
func (a *App) ToggleChannel(channelID string) error {
	ch, err := a.channelManager.Get(channelID)
	if err != nil {
		return err
	}

	if ch.Status == channel.StatusActive {
		return a.StopChannel(channelID)
	}
	return a.StartChannel(channelID)
}

// GetLogs retorna los logs recientes
func (a *App) GetLogs() []LogEntry {
	a.logMutex.RLock()
	defer a.logMutex.RUnlock()

	logs := make([]LogEntry, len(a.logBuffer))
	copy(logs, a.logBuffer)
	return logs
}

// ClearLogs limpia los logs
func (a *App) ClearLogs() {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()

	a.logBuffer = make([]LogEntry, 0, 1000)
}

// GetConfig retorna la configuración actual
func (a *App) GetConfig() *config.Config {
	return a.config
}

// UpdateConfig actualiza la configuración
func (a *App) UpdateConfig(cfg *config.Config) error {
	a.config = cfg
	err := config.Save(cfg)
	if err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Error guardando configuración: %v", err), "")
		return err
	}

	a.AddLog("INFO", "Configuración actualizada", "")
	return nil
}

// GetConnectedClients retorna los clientes WebSocket conectados
func (a *App) GetConnectedClients() []websocket.ClientInfo {
	return a.wsServer.GetClients()
}

// SelectVideoPath abre un diálogo para seleccionar un archivo de video
func (a *App) SelectVideoPath() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Seleccionar archivo de video",
		Filters: []runtime.FileFilter{
			{DisplayName: "Videos", Pattern: "*.mp4;*.avi;*.mkv;*.mov;*.wmv;*.flv"},
			{DisplayName: "Todos los archivos", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// SelectDirectory abre un diálogo para seleccionar un directorio
func (a *App) SelectDirectory() (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Seleccionar directorio de videos",
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// GetVideoFiles retorna los archivos de video en un directorio
func (a *App) GetVideoFiles(dirPath string) ([]string, error) {
	var videos []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := filepath.Ext(path)
			validExts := []string{".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv"}
			for _, validExt := range validExts {
				if ext == validExt {
					videos = append(videos, path)
					break
				}
			}
		}
		return nil
	})

	return videos, err
}

// PlayVideoOnChannel reproduce un video específico en un canal
func (a *App) PlayVideoOnChannel(channelID, videoPath string) error {
	ch, err := a.channelManager.Get(channelID)
	if err != nil {
		return err
	}

	// Si el canal está activo, detenerlo primero
	if ch.Status == channel.StatusActive {
		a.ffmpegManager.Stop(channelID)
	}

	// Actualizar la ruta del video
	a.channelManager.SetCurrentFile(channelID, videoPath)

	// Parsear resolución del canal
	width, height := 1920, 1080 // Valores por defecto
	if ch.Resolution != "" {
		fmt.Sscanf(ch.Resolution, "%dx%d", &width, &height)
	}

	// Usar FPS del canal o el por defecto
	frameRate := ch.FrameRate
	if frameRate == 0 {
		frameRate = a.config.DefaultFrameRate
	}

	// Iniciar con el nuevo video (SRT)
	ffmpegConfig := ffmpeg.StreamConfig{
		ChannelID:     ch.ID,
		InputPath:     videoPath,
		SRTStreamName: ch.SRTStreamName,
		SRTPort:       ch.SRTPort,
		SRTHost:       ch.SRTHost,
		VideoBitrate:  a.config.DefaultVideoBitrate,
		AudioBitrate:  a.config.DefaultAudioBitrate,
		FrameRate:     frameRate,
		Width:         width,
		Height:        height,
		Loop:          true,
	}

	a.AddLog("INFO", fmt.Sprintf("Iniciando FFmpeg: %dx%d @ %dfps en %s:%d", width, height, frameRate, ch.SRTHost, ch.SRTPort), channelID)

	err = a.ffmpegManager.StartWithFallback(ffmpegConfig)
	if err != nil {
		a.channelManager.SetStatus(channelID, channel.StatusError)
		a.AddLog("ERROR", fmt.Sprintf("Error reproduciendo video: %v", err), channelID)
		return err
	}

	a.channelManager.SetStatus(channelID, channel.StatusActive)
	a.AddLog("INFO", fmt.Sprintf("Reproduciendo: %s en canal %s (SRT puerto %d)", filepath.Base(videoPath), ch.Label, ch.SRTPort), channelID)

	runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
		"channelId":   channelID,
		"status":      channel.StatusActive,
		"currentFile": videoPath,
		"srtPort":     ch.SRTPort,
	})

	return nil
}

// ==================== Métodos internos ====================

// AddLog agrega una entrada al log
func (a *App) AddLog(level, message, channelID string) {
	entry := LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     level,
		Message:   message,
		ChannelID: channelID,
	}

	a.logMutex.Lock()
	// Pool de logs con máximo configurable (default 1000)
	// Cuando se excede el límite, se elimina el log más antiguo (índice 0) para optimizar memoria
	maxLogs := 1000
	if a.config != nil && a.config.MaxLogLines > 0 {
		maxLogs = a.config.MaxLogLines
	}
	if len(a.logBuffer) >= maxLogs {
		// Eliminar el primer elemento (índice 0) desplazando el slice
		a.logBuffer = a.logBuffer[1:]
	}
	a.logBuffer = append(a.logBuffer, entry)
	a.logMutex.Unlock()

	// Emitir evento al frontend
	runtime.EventsEmit(a.ctx, "log:new", entry)

	// Log a consola también
	log.Printf("[%s] %s", level, message)
}

// handleWebSocketMessage maneja mensajes WebSocket de clientes Aximmetry
func (a *App) handleWebSocketMessage(clientID string, message []byte) []byte {
	// Log del mensaje raw para debug
	a.AddLog("DEBUG", fmt.Sprintf("WebSocket raw message: %s", string(message)), "")

	// Manejar formato Socket.IO (prefijos numéricos como "42")
	msgStr := string(message)
	jsonStart := strings.Index(msgStr, "{")
	if jsonStart > 0 {
		// Hay un prefijo antes del JSON, eliminarlo
		a.AddLog("DEBUG", fmt.Sprintf("Detectado prefijo Socket.IO: %s", msgStr[:jsonStart]), "")
		message = []byte(msgStr[jsonStart:])
	}

	var msg websocket.Message
	if err := json.Unmarshal(message, &msg); err != nil {
		a.AddLog("ERROR", fmt.Sprintf("Error parseando mensaje WebSocket: %v", err), "")
		return websocket.ErrorResponse("invalid_message", "Error parseando mensaje")
	}

	a.AddLog("INFO", fmt.Sprintf("WebSocket [%s] acción: %s", clientID, msg.Action), msg.ChannelID)

	switch msg.Action {
	case "play_video":
		// Aximmetry solicita reproducir un video específico
		return a.handlePlayVideoRequest(clientID, msg)
	case "play":
		return a.handlePlayRequest(clientID, msg)
	case "stop":
		return a.handleStopRequest(clientID, msg)
	case "status":
		return a.handleStatusRequest(clientID, msg)
	case "list_channels":
		return a.handleListChannelsRequest(clientID)
	case "list_files":
		return a.handleListFilesRequest(clientID, msg)
	default:
		return websocket.ErrorResponse("unknown_action", "Acción desconocida")
	}
}

// handlePlayVideoRequest maneja solicitudes directas de Aximmetry para reproducir un video
// Este es el flujo principal: Aximmetry envía la ruta del video que quiere ver
func (a *App) handlePlayVideoRequest(clientID string, msg websocket.Message) []byte {
	a.AddLog("DEBUG", fmt.Sprintf("handlePlayVideoRequest: filePath=%s, channelId=%s", msg.FilePath, msg.ChannelID), "")

	// Validar que se proporcionó una ruta de video
	if msg.FilePath == "" {
		a.AddLog("ERROR", "FilePath vacío en solicitud play_video", "")
		return websocket.ErrorResponse("missing_file_path", "Se requiere la ruta del video (filePath)")
	}

	// Verificar que el archivo existe
	if _, err := os.Stat(msg.FilePath); os.IsNotExist(err) {
		a.AddLog("ERROR", fmt.Sprintf("Archivo no encontrado: %s", msg.FilePath), "")
		return websocket.ErrorResponse("file_not_found", fmt.Sprintf("Archivo no encontrado: %s", msg.FilePath))
	}

	a.AddLog("DEBUG", fmt.Sprintf("Archivo verificado: %s", msg.FilePath), "")

	// Determinar el canal a usar
	var channelID string
	var streamName string
	var srtPort int
	var srtHost string

	if msg.ChannelID != "" {
		// Si se especifica un canal, buscarlo por ID o por Label
		ch, err := a.channelManager.Get(msg.ChannelID)
		if err != nil {
			// Buscar por label si no se encontró por ID
			ch = a.channelManager.GetByLabel(msg.ChannelID)
			if ch == nil {
				a.AddLog("ERROR", fmt.Sprintf("Canal no encontrado: %s", msg.ChannelID), "")
				return websocket.ErrorResponse("channel_not_found", fmt.Sprintf("Canal '%s' no encontrado. Crea el canal primero o no envíes channelId para crear uno automático.", msg.ChannelID))
			}
		}
		channelID = ch.ID
		streamName = ch.SRTStreamName
		srtPort = ch.SRTPort
		srtHost = ch.SRTHost
		a.AddLog("DEBUG", fmt.Sprintf("Usando canal existente: %s (SRT %s:%d)", ch.Label, srtHost, srtPort), channelID)
	} else {
		// Crear o reutilizar un canal para este cliente
		// Buscar si el cliente ya tiene un canal asignado
		channels := a.channelManager.GetAll()
		for _, ch := range channels {
			if ch.Label == "Client_"+clientID {
				channelID = ch.ID
				streamName = ch.SRTStreamName
				srtPort = ch.SRTPort
				srtHost = ch.SRTHost
				break
			}
		}

		// Si no existe, crear uno nuevo para este cliente
		if channelID == "" {
			newStreamName := fmt.Sprintf("%s%s", a.config.SRTPrefix, clientID[:8])
			ch, err := a.channelManager.Add("Client_"+clientID, msg.FilePath, newStreamName)
			if err != nil {
				a.AddLog("ERROR", fmt.Sprintf("Error creando canal para cliente: %v", err), "")
				return websocket.ErrorResponse("channel_create_error", err.Error())
			}
			channelID = ch.ID
			streamName = ch.SRTStreamName
			srtPort = ch.SRTPort
			srtHost = ch.SRTHost

			// Notificar al frontend del nuevo canal
			runtime.EventsEmit(a.ctx, "channel:added", ch)
			a.AddLog("INFO", fmt.Sprintf("Canal creado automáticamente para cliente %s: SRT %s:%d", clientID[:8], srtHost, srtPort), channelID)
		}
	}

	// Reproducir el video solicitado
	err := a.PlayVideoOnChannel(channelID, msg.FilePath)
	if err != nil {
		return websocket.ErrorResponse("play_error", err.Error())
	}

	// Usar el SRTHost configurado en el canal, o detectar automáticamente si es 0.0.0.0
	displayHost := srtHost
	if displayHost == "" || displayHost == "0.0.0.0" {
		displayHost = a.getServerIP()
	}
	srtURL := fmt.Sprintf("srt://%s:%d", displayHost, srtPort)

	a.AddLog("INFO", fmt.Sprintf("Aximmetry [%s] solicitó: %s -> %s", clientID[:8], filepath.Base(msg.FilePath), srtURL), channelID)

	return websocket.SuccessResponse("play_started", map[string]interface{}{
		"channelId":  channelID,
		"streamName": streamName,
		"srtPort":    srtPort,
		"srtHost":    srtHost,
		"srtUrl":     srtURL,
		"filePath":   msg.FilePath,
		"message":    fmt.Sprintf("Video disponible en: %s", srtURL),
	})
}

func (a *App) handlePlayRequest(clientID string, msg websocket.Message) []byte {
	// Verificar que el canal existe
	ch, err := a.channelManager.Get(msg.ChannelID)
	if err != nil {
		return websocket.ErrorResponse("channel_not_found", "Canal no encontrado")
	}

	videoPath := msg.FilePath
	if videoPath == "" {
		videoPath = ch.VideoPath
	}

	// Iniciar reproducción
	err = a.PlayVideoOnChannel(msg.ChannelID, videoPath)
	if err != nil {
		return websocket.ErrorResponse("play_error", err.Error())
	}

	serverIP := a.getServerIP()
	srtURL := fmt.Sprintf("srt://%s:%d", serverIP, ch.SRTPort)

	return websocket.SuccessResponse("play_started", map[string]interface{}{
		"channelId":  ch.ID,
		"streamName": ch.SRTStreamName,
		"srtPort":    ch.SRTPort,
		"srtUrl":     srtURL,
		"filePath":   videoPath,
	})
}

func (a *App) handleStopRequest(clientID string, msg websocket.Message) []byte {
	err := a.StopChannel(msg.ChannelID)
	if err != nil {
		return websocket.ErrorResponse("stop_error", err.Error())
	}

	return websocket.SuccessResponse("play_stopped", map[string]interface{}{
		"channelId": msg.ChannelID,
	})
}

func (a *App) handleStatusRequest(clientID string, msg websocket.Message) []byte {
	if msg.ChannelID != "" {
		ch, err := a.channelManager.Get(msg.ChannelID)
		if err != nil {
			return websocket.ErrorResponse("channel_not_found", "Canal no encontrado")
		}
		return websocket.SuccessResponse("channel_status", ch)
	}

	// Retornar estado de todos los canales
	channels := a.channelManager.GetAll()
	return websocket.SuccessResponse("all_channels_status", channels)
}

func (a *App) handleListChannelsRequest(clientID string) []byte {
	channels := a.channelManager.GetAll()
	return websocket.SuccessResponse("channels_list", channels)
}

func (a *App) handleListFilesRequest(clientID string, msg websocket.Message) []byte {
	ch, err := a.channelManager.Get(msg.ChannelID)
	if err != nil {
		return websocket.ErrorResponse("channel_not_found", "Canal no encontrado")
	}

	files, err := a.GetVideoFiles(filepath.Dir(ch.VideoPath))
	if err != nil {
		return websocket.ErrorResponse("list_error", err.Error())
	}

	return websocket.SuccessResponse("files_list", files)
}

// onFFmpegEvent maneja eventos del gestor FFmpeg
func (a *App) onFFmpegEvent(event ffmpeg.Event) {
	var newStatus channel.Status

	switch event.Type {
	case ffmpeg.EventStarted:
		a.AddLog("INFO", fmt.Sprintf("FFmpeg iniciado para canal %s", event.ChannelID), event.ChannelID)
		// No cambiar status aquí - ya se hizo en PlayTestPattern/StartChannel
		return // No emitir evento duplicado
	case ffmpeg.EventWarning:
		// Encoder de hardware no disponible, usando fallback
		a.AddLog("WARNING", event.Message, event.ChannelID)
		runtime.EventsEmit(a.ctx, "ffmpeg:warning", map[string]interface{}{
			"channelId": event.ChannelID,
			"message":   event.Message,
			"data":      event.Data,
		})
		return // No cambiar status, el stream continuará con el fallback
	case ffmpeg.EventStopped:
		a.AddLog("INFO", fmt.Sprintf("FFmpeg detenido para canal %s", event.ChannelID), event.ChannelID)
		a.channelManager.SetStatus(event.ChannelID, channel.StatusInactive)
		newStatus = channel.StatusInactive
	case ffmpeg.EventError:
		// Detectar si es un error de desconexión del cliente SRT (I/O error)
		isSRTDisconnect := strings.Contains(event.Message, "I/O error") ||
			strings.Contains(event.Message, "exit status 0xfffffffb") ||
			strings.Contains(event.Message, "muxing a packet")

		if isSRTDisconnect {
			a.AddLog("INFO", fmt.Sprintf("Cliente SRT desconectado del canal %s. Pulse 'Patrón' o 'Iniciar' para reanudar.", event.ChannelID), event.ChannelID)
			a.channelManager.SetStatus(event.ChannelID, channel.StatusInactive)
			newStatus = channel.StatusInactive
		} else {
			a.AddLog("ERROR", fmt.Sprintf("Error FFmpeg en canal %s: %s", event.ChannelID, event.Message), event.ChannelID)
			a.channelManager.SetStatus(event.ChannelID, channel.StatusError)
			newStatus = channel.StatusError

			// Intentar reinicio automático si está configurado
			if a.config.AutoRestart {
				go a.attemptRestart(event.ChannelID)
			}
		}
	default:
		return
	}

	// Emitir channel:status con el status actualizado
	runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
		"channelId": event.ChannelID,
		"status":    newStatus,
		"event":     event.Type,
		"message":   event.Message,
	})
}

// attemptRestart intenta reiniciar un canal que falló
// Solo reinicia si hay un archivo para reproducir y no excede el límite de reintentos
func (a *App) attemptRestart(channelID string) {
	// No reintentar inmediatamente, usar backoff
	time.Sleep(10 * time.Second)

	ch, err := a.channelManager.Get(channelID)
	if err != nil {
		return
	}

	// No reiniciar si no está en error o si no hay archivo configurado
	if ch.Status != channel.StatusError {
		return
	}

	// Verificar que hay un archivo para reproducir
	inputPath := ch.VideoPath
	if inputPath == "" {
		inputPath = ch.CurrentFile
	}

	if inputPath == "" {
		a.AddLog("INFO", fmt.Sprintf("Canal %s en error pero sin archivo para reiniciar. Use 'Patrón' o configure un video.", ch.Label), channelID)
		return
	}

	// Verificar que el archivo existe antes de reintentar
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		a.AddLog("ERROR", fmt.Sprintf("Archivo no encontrado para reinicio: %s", inputPath), channelID)
		return
	}

	a.AddLog("INFO", fmt.Sprintf("Intentando reiniciar canal %s", ch.Label), channelID)
	a.StartChannel(channelID)
}

// getServerIP obtiene la IP local del servidor
func (a *App) getServerIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "localhost"
}

// monitorChannels monitorea el estado de todos los canales
func (a *App) monitorChannels(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			channels := a.channelManager.GetAll()
			for _, ch := range channels {
				if ch.Status == channel.StatusActive {
					// Verificar que FFmpeg sigue corriendo
					if !a.ffmpegManager.IsRunning(ch.ID) {
						a.channelManager.SetStatus(ch.ID, channel.StatusInactive)
						runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
							"channelId": ch.ID,
							"status":    channel.StatusInactive,
						})
					}
				}
			}
		}
	}
}
