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
	Loop          bool
	// Configuración avanzada de encoding
	VideoEncoder   string // libx264, h264_nvenc, h264_qsv, h264_amf
	EncoderPreset  string // ultrafast, veryfast, fast, medium
	EncoderProfile string // baseline, main, high
	EncoderTune    string // zerolatency, film, animation
	GopSize        int    // Keyframe interval
	BFrames        int    // B-frames
	// Control de bitrate
	BitrateMode string // cbr, vbr
	MaxBitrate  string
	BufferSize  string
	// SRT avanzado
	SRTLatency    int // ms
	SRTRecvBuffer int // bytes
	SRTSendBuffer int // bytes
	SRTOverheadBW int // porcentaje
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

	// Determinar el encoder a usar
	encoder := config.VideoEncoder
	if encoder == "" {
		encoder = "libx264"
	}

	// Opciones de loop
	if config.Loop {
		args = append(args, "-stream_loop", "-1")
	}

	// Input
	args = append(args,
		"-re",                // Sincronización de tiempo real
		"-fflags", "+genpts", // Generar timestamps correctos
		"-i", config.InputPath,
	)

	// === Encoder de Video ===
	args = append(args, "-c:v", encoder)

	// Configuración específica por encoder
	switch encoder {
	case "h264_nvenc":
		// NVIDIA NVENC - Aceleración por hardware (RTX 3060 Ti y similares)
		// La RTX 3060 Ti puede manejar múltiples sesiones NVENC simultáneas (hasta ~5-8)
		preset := config.EncoderPreset
		if preset == "" {
			preset = "p4" // Balance velocidad/calidad para NVENC
		} else {
			// Mapear presets de libx264 a NVENC
			nvencPresets := map[string]string{
				"ultrafast": "p1",
				"veryfast":  "p2",
				"fast":      "p4",
				"medium":    "p5",
			}
			if p, ok := nvencPresets[preset]; ok {
				preset = p
			}
		}
		args = append(args,
			"-preset", preset,
			"-tune", "ll", // Low latency (ultra low latency)
			"-rc", "cbr", // CBR para streaming estable
			"-rc-lookahead", "0", // Sin lookahead para baja latencia
		)
		// Profile
		profile := config.EncoderProfile
		if profile == "" {
			profile = "main"
		}
		args = append(args, "-profile:v", profile)

	case "h264_qsv":
		// Intel QuickSync
		preset := config.EncoderPreset
		if preset == "" {
			preset = "veryfast"
		}
		args = append(args,
			"-preset", preset,
			"-look_ahead", "0", // Deshabilitar lookahead para baja latencia
		)
		if config.EncoderProfile != "" {
			args = append(args, "-profile:v", config.EncoderProfile)
		}

	case "h264_amf":
		// AMD AMF
		args = append(args,
			"-quality", "speed",
			"-rc", "cbr",
		)
		if config.EncoderProfile != "" {
			args = append(args, "-profile:v", config.EncoderProfile)
		}

	default:
		// libx264 (CPU)
		preset := config.EncoderPreset
		if preset == "" {
			preset = "veryfast"
		}
		profile := config.EncoderProfile
		if profile == "" {
			profile = "main"
		}
		args = append(args,
			"-profile:v", profile,
			"-level", "4.0",
			"-preset", preset,
		)
		// Tune solo para libx264
		tune := config.EncoderTune
		if tune == "" {
			tune = "zerolatency"
		}
		if tune != "" {
			args = append(args, "-tune", tune)
		}
		// Opciones específicas de libx264 para baja latencia
		args = append(args,
			"-refs", "1", // Una sola referencia
			"-nal-hrd", "cbr", // CBR estricto
		)
	}

	// === GOP y B-Frames (común a todos los encoders) ===
	gopSize := config.GopSize
	if gopSize <= 0 {
		gopSize = 50 // 2 segundos a 25fps por defecto
	}
	args = append(args,
		"-g", strconv.Itoa(gopSize),
		"-keyint_min", strconv.Itoa(gopSize),
		"-sc_threshold", "0", // Deshabilitar detección de cambio de escena
	)

	bframes := config.BFrames
	args = append(args, "-bf", strconv.Itoa(bframes))

	// === Control de Bitrate ===
	videoBitrate := config.VideoBitrate
	if videoBitrate == "" {
		videoBitrate = "5M"
	}
	args = append(args, "-b:v", videoBitrate)

	maxBitrate := config.MaxBitrate
	if maxBitrate == "" {
		maxBitrate = videoBitrate // CBR: maxrate = bitrate
	}
	bufSize := config.BufferSize
	if bufSize == "" {
		bufSize = videoBitrate // CBR: bufsize = bitrate
	}
	args = append(args, "-maxrate", maxBitrate, "-bufsize", bufSize)

	// === Resolución ===
	if config.Width > 0 && config.Height > 0 {
		args = append(args, "-s", fmt.Sprintf("%dx%d", config.Width, config.Height))
	}

	// === Frame Rate ===
	if config.FrameRate > 0 {
		args = append(args, "-r", strconv.Itoa(config.FrameRate))
	}

	// Formato de pixel (necesario para compatibilidad con NVENC)
	args = append(args, "-pix_fmt", "yuv420p")

	// === Audio ===
	args = append(args,
		"-c:a", "aac",
		"-ar", "48000",
		"-ac", "2",
		"-af", "aresample=async=1:min_hard_comp=0.100000:first_pts=0",
	)

	audioBitrate := config.AudioBitrate
	if audioBitrate == "" {
		audioBitrate = "192k"
	}
	args = append(args, "-b:a", audioBitrate)

	// === Output SRT ===
	srtPort := config.SRTPort
	if srtPort == 0 {
		srtPort = 9000
	}

	srtHost := config.SRTHost
	if srtHost == "" {
		srtHost = "0.0.0.0"
	}

	// Parámetros SRT
	srtLatency := config.SRTLatency
	if srtLatency <= 0 {
		srtLatency = 500 // 500ms por defecto
	}
	srtLatencyUs := srtLatency * 1000 // Convertir a microsegundos

	srtRecvBuf := config.SRTRecvBuffer
	if srtRecvBuf <= 0 {
		srtRecvBuf = 8388608 // 8MB por defecto
	}

	srtSendBuf := config.SRTSendBuffer
	if srtSendBuf <= 0 {
		srtSendBuf = 8388608 // 8MB por defecto
	}

	srtOverhead := config.SRTOverheadBW
	if srtOverhead <= 0 {
		srtOverhead = 25 // 25% por defecto
	}

	// Calcular muxrate basado en bitrate
	muxrate := "8M"

	// Construir URL SRT con parámetros configurables
	srtURL := fmt.Sprintf(
		"srt://%s:%d?mode=listener&latency=%d&pkt_size=1316&rcvbuf=%d&sndbuf=%d&maxbw=-1&oheadbw=%d&listen_timeout=-1",
		srtHost, srtPort, srtLatencyUs, srtRecvBuf, srtSendBuf, srtOverhead,
	)

	args = append(args,
		"-f", "mpegts",
		"-mpegts_copyts", "1",
		"-muxrate", muxrate,
		"-pcr_period", "20",
		srtURL,
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
