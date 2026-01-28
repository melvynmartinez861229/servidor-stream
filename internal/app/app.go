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

	// Configurar y iniciar FFmpeg con SRT
	ffmpegConfig := ffmpeg.StreamConfig{
		ChannelID:     ch.ID,
		InputPath:     inputPath,
		SRTStreamName: ch.SRTStreamName,
		SRTPort:       ch.SRTPort,
		VideoBitrate:  a.config.DefaultVideoBitrate,
		AudioBitrate:  a.config.DefaultAudioBitrate,
		FrameRate:     a.config.DefaultFrameRate,
		Loop:          true, // Loop por defecto
	}

	err = a.ffmpegManager.Start(ffmpegConfig)
	if err != nil {
		a.channelManager.SetStatus(channelID, channel.StatusError)
		a.AddLog("ERROR", fmt.Sprintf("Error iniciando stream %s: %v", ch.Label, err), channelID)
		return err
	}

	a.channelManager.SetStatus(channelID, channel.StatusActive)
	a.AddLog("INFO", fmt.Sprintf("Stream SRT iniciado: %s -> srt://0.0.0.0:%d", ch.Label, ch.SRTPort), channelID)
	runtime.EventsEmit(a.ctx, "channel:status", map[string]interface{}{
		"channelId": channelID,
		"status":    channel.StatusActive,
		"srtPort":   ch.SRTPort,
	})

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

	// Configurar y iniciar FFmpeg con el patrón
	ffmpegConfig := ffmpeg.StreamConfig{
		ChannelID:     ch.ID,
		InputPath:     a.config.TestPatternPath,
		SRTStreamName: ch.SRTStreamName,
		SRTPort:       ch.SRTPort,
		VideoBitrate:  a.config.DefaultVideoBitrate,
		AudioBitrate:  a.config.DefaultAudioBitrate,
		FrameRate:     a.config.DefaultFrameRate,
		Loop:          true, // El patrón siempre en loop
	}

	err = a.ffmpegManager.Start(ffmpegConfig)
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

	// Iniciar con el nuevo video (SRT)
	ffmpegConfig := ffmpeg.StreamConfig{
		ChannelID:     ch.ID,
		InputPath:     videoPath,
		SRTStreamName: ch.SRTStreamName,
		SRTPort:       ch.SRTPort,
		VideoBitrate:  a.config.DefaultVideoBitrate,
		AudioBitrate:  a.config.DefaultAudioBitrate,
		FrameRate:     a.config.DefaultFrameRate,
		Loop:          true,
	}

	err = a.ffmpegManager.Start(ffmpegConfig)
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
	a.logBuffer = append(a.logBuffer, entry)
	// Mantener solo los últimos 1000 logs
	if len(a.logBuffer) > 1000 {
		a.logBuffer = a.logBuffer[100:]
	}
	a.logMutex.Unlock()

	// Emitir evento al frontend
	runtime.EventsEmit(a.ctx, "log:new", entry)

	// Log a consola también
	log.Printf("[%s] %s", level, message)
}

// handleWebSocketMessage maneja mensajes WebSocket de clientes Aximmetry
func (a *App) handleWebSocketMessage(clientID string, message []byte) []byte {
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
	// Validar que se proporcionó una ruta de video
	if msg.FilePath == "" {
		return websocket.ErrorResponse("missing_file_path", "Se requiere la ruta del video (filePath)")
	}

	// Verificar que el archivo existe
	if _, err := os.Stat(msg.FilePath); os.IsNotExist(err) {
		a.AddLog("ERROR", fmt.Sprintf("Archivo no encontrado: %s", msg.FilePath), "")
		return websocket.ErrorResponse("file_not_found", fmt.Sprintf("Archivo no encontrado: %s", msg.FilePath))
	}

	// Determinar el canal a usar
	var channelID string
	var streamName string
	var srtPort int

	if msg.ChannelID != "" {
		// Si se especifica un canal, usarlo
		ch, err := a.channelManager.Get(msg.ChannelID)
		if err != nil {
			return websocket.ErrorResponse("channel_not_found", "Canal no encontrado")
		}
		channelID = ch.ID
		streamName = ch.SRTStreamName
		srtPort = ch.SRTPort
	} else {
		// Crear o reutilizar un canal para este cliente
		// Buscar si el cliente ya tiene un canal asignado
		channels := a.channelManager.GetAll()
		for _, ch := range channels {
			if ch.Label == "Client_"+clientID {
				channelID = ch.ID
				streamName = ch.SRTStreamName
				srtPort = ch.SRTPort
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

			// Notificar al frontend del nuevo canal
			runtime.EventsEmit(a.ctx, "channel:added", ch)
			a.AddLog("INFO", fmt.Sprintf("Canal creado automáticamente para cliente %s: SRT puerto %d", clientID[:8], srtPort), channelID)
		}
	}

	// Reproducir el video solicitado
	err := a.PlayVideoOnChannel(channelID, msg.FilePath)
	if err != nil {
		return websocket.ErrorResponse("play_error", err.Error())
	}

	// Obtener la IP del servidor para la URL SRT
	serverIP := a.getServerIP()
	srtURL := fmt.Sprintf("srt://%s:%d", serverIP, srtPort)

	a.AddLog("INFO", fmt.Sprintf("Aximmetry [%s] solicitó: %s -> %s", clientID[:8], filepath.Base(msg.FilePath), srtURL), channelID)

	return websocket.SuccessResponse("play_started", map[string]interface{}{
		"channelId":  channelID,
		"streamName": streamName,
		"srtPort":    srtPort,
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
	case ffmpeg.EventProgress:
		// Actualizar progreso en el canal
		runtime.EventsEmit(a.ctx, "channel:progress", map[string]interface{}{
			"channelId": event.ChannelID,
			"progress":  event.Data,
		})
		return // No emitir channel:status para progreso
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
