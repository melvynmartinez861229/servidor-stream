package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// EventType tipo de evento FFmpeg
type EventType string

const (
	EventStarted  EventType = "started"
	EventStopped  EventType = "stopped"
	EventError    EventType = "error"
	EventProgress EventType = "progress"
)

// Event representa un evento del proceso FFmpeg
type Event struct {
	Type      EventType
	ChannelID string
	Message   string
	Data      map[string]interface{}
}

// StreamConfig configuración para un stream SRT
type StreamConfig struct {
	ChannelID     string
	InputPath     string
	SRTStreamName string // Nombre identificador del stream SRT
	SRTPort       int    // Puerto SRT para este canal
	SRTHost       string // IP/Host para el stream SRT
	VideoBitrate  string
	AudioBitrate  string
	FrameRate     int
	Width         int
	Height        int
	PixelFormat   string
	Loop          bool
}

// ProcessInfo información de un proceso FFmpeg
type ProcessInfo struct {
	ChannelID    string
	PID          int
	StartTime    time.Time
	Config       StreamConfig
	IsRunning    bool
	Progress     Progress
	LastError    string
	RestartCount int
}

// Progress progreso del proceso FFmpeg
type Progress struct {
	Frame      int64   `json:"frame"`
	FPS        float64 `json:"fps"`
	Bitrate    string  `json:"bitrate"`
	TotalSize  int64   `json:"totalSize"`
	OutTime    string  `json:"outTime"`
	Speed      string  `json:"speed"`
	DupFrames  int64   `json:"dupFrames"`
	DropFrames int64   `json:"dropFrames"`
}

// Manager gestor de procesos FFmpeg
type Manager struct {
	ffmpegPath   string
	processes    map[string]*ffmpegProcess
	mutex        sync.RWMutex
	eventHandler func(Event)
}

type ffmpegProcess struct {
	config       StreamConfig
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	startTime    time.Time
	progress     Progress
	lastError    string
	restartCount int
	stderr       io.ReadCloser
	stopped      bool // Marcado como detenido intencionalmente
}

// NewManager crea un nuevo gestor de procesos FFmpeg
func NewManager(ffmpegPath string, eventHandler func(Event)) *Manager {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg" // Usar FFmpeg del PATH
	}

	return &Manager{
		ffmpegPath:   ffmpegPath,
		processes:    make(map[string]*ffmpegProcess),
		eventHandler: eventHandler,
	}
}

// Start inicia un proceso FFmpeg para streaming SRT
func (m *Manager) Start(config StreamConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Verificar si ya existe un proceso para este canal
	if proc, exists := m.processes[config.ChannelID]; exists {
		if proc.cmd != nil && proc.cmd.Process != nil {
			return fmt.Errorf("ya existe un proceso activo para el canal %s", config.ChannelID)
		}
	}

	// Verificar que el archivo de entrada existe
	if _, err := os.Stat(config.InputPath); os.IsNotExist(err) {
		return fmt.Errorf("archivo de entrada no encontrado: %s", config.InputPath)
	}

	// Construir argumentos de FFmpeg
	args := m.buildFFmpegArgs(config)

	// Crear contexto con cancelación
	ctx, cancel := context.WithCancel(context.Background())

	// Crear comando
	cmd := exec.CommandContext(ctx, m.ffmpegPath, args...)

	// Ocultar ventana de consola en Windows
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	// Capturar stderr para progreso
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("error creando pipe stderr: %v", err)
	}

	// Iniciar proceso
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("error iniciando FFmpeg: %v", err)
	}

	proc := &ffmpegProcess{
		config:    config,
		cmd:       cmd,
		cancel:    cancel,
		startTime: time.Now(),
		stderr:    stderr,
	}

	m.processes[config.ChannelID] = proc

	// Monitorear proceso en goroutine
	go m.monitorProcess(config.ChannelID, proc)

	// Emitir evento de inicio
	srtPort := config.SRTPort
	if srtPort == 0 {
		srtPort = 9000
	}
	m.emitEvent(Event{
		Type:      EventStarted,
		ChannelID: config.ChannelID,
		Message:   fmt.Sprintf("Stream SRT iniciado en puerto %d", srtPort),
		Data: map[string]interface{}{
			"pid":        cmd.Process.Pid,
			"streamName": config.SRTStreamName,
			"inputPath":  config.InputPath,
			"srtPort":    srtPort,
			"srtUrl":     fmt.Sprintf("srt://IP_SERVIDOR:%d", srtPort),
		},
	})

	return nil
}

// Stop detiene un proceso FFmpeg
func (m *Manager) Stop(channelID string) error {
	m.mutex.Lock()
	proc, exists := m.processes[channelID]
	if exists {
		// Marcar como detenido intencionalmente para que monitorProcess no emita eventos
		proc.stopped = true
	}
	m.mutex.Unlock()

	if !exists {
		return nil // No hay proceso, no es error
	}

	// Cancelar contexto (termina el proceso)
	if proc.cancel != nil {
		proc.cancel()
	}

	// Esperar para terminación limpia y liberación del puerto
	time.Sleep(1 * time.Second)

	// Forzar terminación si sigue corriendo
	if proc.cmd != nil && proc.cmd.Process != nil {
		proc.cmd.Process.Kill()
	}

	// Esperar un poco más para que el puerto se libere
	time.Sleep(500 * time.Millisecond)

	m.mutex.Lock()
	delete(m.processes, channelID)
	m.mutex.Unlock()

	// Emitir evento de detención (solo si realmente se detuvo)
	m.emitEvent(Event{
		Type:      EventStopped,
		ChannelID: channelID,
		Message:   "Stream detenido",
	})

	return nil
}

// StopAll detiene todos los procesos FFmpeg
func (m *Manager) StopAll() {
	m.mutex.RLock()
	channelIDs := make([]string, 0, len(m.processes))
	for id := range m.processes {
		channelIDs = append(channelIDs, id)
	}
	m.mutex.RUnlock()

	for _, id := range channelIDs {
		m.Stop(id)
	}
}

// IsRunning verifica si un proceso está corriendo
func (m *Manager) IsRunning(channelID string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	proc, exists := m.processes[channelID]
	if !exists {
		return false
	}

	if proc.cmd == nil || proc.cmd.Process == nil {
		return false
	}

	// Verificar si el proceso sigue corriendo
	return proc.cmd.ProcessState == nil || !proc.cmd.ProcessState.Exited()
}

// GetProcessInfo obtiene información de un proceso
func (m *Manager) GetProcessInfo(channelID string) (*ProcessInfo, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	proc, exists := m.processes[channelID]
	if !exists {
		return nil, fmt.Errorf("proceso no encontrado")
	}

	pid := 0
	if proc.cmd != nil && proc.cmd.Process != nil {
		pid = proc.cmd.Process.Pid
	}

	return &ProcessInfo{
		ChannelID:    channelID,
		PID:          pid,
		StartTime:    proc.startTime,
		Config:       proc.config,
		IsRunning:    m.IsRunning(channelID),
		Progress:     proc.progress,
		LastError:    proc.lastError,
		RestartCount: proc.restartCount,
	}, nil
}

// GetAllProcessInfo obtiene información de todos los procesos
func (m *Manager) GetAllProcessInfo() []ProcessInfo {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	infos := make([]ProcessInfo, 0, len(m.processes))
	for channelID, proc := range m.processes {
		pid := 0
		if proc.cmd != nil && proc.cmd.Process != nil {
			pid = proc.cmd.Process.Pid
		}

		infos = append(infos, ProcessInfo{
			ChannelID:    channelID,
			PID:          pid,
			StartTime:    proc.startTime,
			Config:       proc.config,
			IsRunning:    proc.cmd != nil && proc.cmd.ProcessState == nil,
			Progress:     proc.progress,
			LastError:    proc.lastError,
			RestartCount: proc.restartCount,
		})
	}

	return infos
}

// buildFFmpegArgs construye los argumentos para FFmpeg con salida SRT
func (m *Manager) buildFFmpegArgs(config StreamConfig) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "info",
		"-stats",
	}

	// Opciones de loop
	if config.Loop {
		args = append(args, "-stream_loop", "-1")
	}

	// Input - sin -re para máxima fluidez (el encoding controla el ritmo)
	args = append(args,
		"-re",                // Mantener para sincronización de tiempo real
		"-fflags", "+genpts", // Generar timestamps correctos
		"-i", config.InputPath,
	)

	// Codec de video - H.264 optimizado para baja latencia
	args = append(args,
		"-c:v", "libx264",
		"-profile:v", "high",
		"-level", "4.1",
		"-preset", "ultrafast", // Más rápido para menor latencia
		"-tune", "zerolatency", // Baja latencia para streaming
		"-g", "30", // Keyframe cada 30 frames (1 segundo a 30fps)
		"-keyint_min", "30",
		"-sc_threshold", "0", // Deshabilitar detección de cambio de escena
		"-bf", "0", // Sin B-frames para menor latencia
	)

	// Bitrate de video
	if config.VideoBitrate != "" {
		args = append(args, "-b:v", config.VideoBitrate)
	} else {
		args = append(args, "-b:v", "5M")
	}
	// Añadir maxrate y bufsize para mejor control de bitrate
	args = append(args, "-maxrate", "6M", "-bufsize", "3M")

	// Resolución (si se especifica)
	if config.Width > 0 && config.Height > 0 {
		args = append(args,
			"-s", fmt.Sprintf("%dx%d", config.Width, config.Height),
		)
	}

	// Frame rate
	if config.FrameRate > 0 {
		args = append(args,
			"-r", strconv.Itoa(config.FrameRate),
		)
	}

	// Formato de pixel
	args = append(args, "-pix_fmt", "yuv420p")

	// Codec de audio - AAC para SRT con configuración optimizada
	// Usamos -af aresample para asegurar compatibilidad y sincronización
	args = append(args,
		"-c:a", "aac",
		"-ar", "48000",
		"-ac", "2",
		"-af", "aresample=async=1:min_hard_comp=0.100000:first_pts=0", // Resincronizar audio
	)

	// Bitrate de audio
	if config.AudioBitrate != "" {
		args = append(args, "-b:a", config.AudioBitrate)
	} else {
		args = append(args, "-b:a", "192k")
	}

	// Output SRT (modo listener - el servidor espera conexiones de Aximmetry como caller)
	srtPort := config.SRTPort
	if srtPort == 0 {
		srtPort = 9000 // Puerto por defecto
	}

	srtHost := config.SRTHost
	if srtHost == "" {
		srtHost = "0.0.0.0" // Por defecto escucha en todas las interfaces
	}

	// Formato MPEG-TS para SRT con parámetros optimizados para BAJA LATENCIA
	// latency=120000 = 120ms (suficiente para local, mucho más fluido)
	// rcvbuf/sndbuf optimizados para menor delay
	args = append(args,
		"-f", "mpegts",
		"-muxrate", "6M", // Tasa de mux fija para estabilidad
		fmt.Sprintf("srt://%s:%d?mode=listener&latency=120000&pkt_size=1316&rcvbuf=1000000&sndbuf=1000000&listen_timeout=-1", srtHost, srtPort),
	)

	return args
}

// buildFFmpegArgsAlt construye argumentos alternativos para raw video
// Se mantiene para compatibilidad con otros formatos de salida
func (m *Manager) buildFFmpegArgsAlt(config StreamConfig) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "info",
		"-stats",
	}

	if config.Loop {
		args = append(args, "-stream_loop", "-1")
	}

	args = append(args,
		"-re",
		"-i", config.InputPath,
		"-c:v", "rawvideo",
		"-pix_fmt", "uyvy422",
	)

	if config.FrameRate > 0 {
		args = append(args, "-r", strconv.Itoa(config.FrameRate))
	}

	args = append(args,
		"-c:a", "pcm_s16le",
		"-ar", "48000",
		"-ac", "2",
		"-f", "nut",
		"pipe:1", // Output a stdout
	)

	return args
}

// monitorProcess monitorea un proceso FFmpeg
func (m *Manager) monitorProcess(channelID string, proc *ffmpegProcess) {
	// Leer stderr para progreso
	go m.parseProgress(channelID, proc)

	// Esperar a que el proceso termine
	err := proc.cmd.Wait()

	m.mutex.Lock()
	// Solo emitir eventos si el proceso NO fue detenido intencionalmente
	if !proc.stopped {
		if _, exists := m.processes[channelID]; exists {
			if err != nil {
				proc.lastError = err.Error()
				m.emitEvent(Event{
					Type:      EventError,
					ChannelID: channelID,
					Message:   err.Error(),
				})
			} else {
				m.emitEvent(Event{
					Type:      EventStopped,
					ChannelID: channelID,
					Message:   "Proceso terminado normalmente",
				})
			}
			delete(m.processes, channelID)
		}
	}
	m.mutex.Unlock()
}

// parseProgress lee la salida de FFmpeg para logging y detección de errores
func (m *Manager) parseProgress(channelID string, proc *ffmpegProcess) {
	scanner := bufio.NewScanner(proc.stderr)

	for scanner.Scan() {
		line := scanner.Text()

		// Log para debugging
		log.Printf("[FFmpeg %s] %s", channelID, line)

		// Detectar errores
		if strings.Contains(strings.ToLower(line), "error") {
			m.emitEvent(Event{
				Type:      EventError,
				ChannelID: channelID,
				Message:   line,
			})
		}
	}
}

// emitEvent emite un evento
func (m *Manager) emitEvent(event Event) {
	if m.eventHandler != nil {
		m.eventHandler(event)
	}
}

// CheckFFmpegInstalled verifica si FFmpeg está instalado
func CheckFFmpegInstalled(ffmpegPath string) (bool, string) {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	cmd := exec.Command(ffmpegPath, "-version")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	// Extraer versión
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return true, strings.TrimSpace(lines[0])
	}

	return true, "unknown"
}

// GetFFmpegFormats obtiene los formatos soportados por FFmpeg
func GetFFmpegFormats(ffmpegPath string) ([]string, error) {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	cmd := exec.Command(ffmpegPath, "-formats")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var formats []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "D") || strings.HasPrefix(line, "E") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				formats = append(formats, parts[1])
			}
		}
	}

	return formats, nil
}

// HasSRTSupport verifica si FFmpeg tiene soporte SRT
func HasSRTSupport(ffmpegPath string) bool {
	formats, err := GetFFmpegFormats(ffmpegPath)
	if err != nil {
		return false
	}

	for _, format := range formats {
		if strings.Contains(format, "srt") || strings.Contains(format, "mpegts") {
			return true
		}
	}

	return false
}
