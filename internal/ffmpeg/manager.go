package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	NDIStreamName string // Ahora se usa como nombre identificador del stream
	SRTPort       int    // Puerto SRT para este canal
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

// Start inicia un proceso FFmpeg para streaming NDI
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
			"streamName": config.NDIStreamName,
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
	m.mutex.Unlock()

	if !exists {
		return fmt.Errorf("no existe proceso para el canal %s", channelID)
	}

	// Cancelar contexto (termina el proceso)
	if proc.cancel != nil {
		proc.cancel()
	}

	// Esperar un poco para terminación limpia
	time.Sleep(500 * time.Millisecond)

	// Forzar terminación si sigue corriendo
	if proc.cmd != nil && proc.cmd.Process != nil {
		proc.cmd.Process.Kill()
	}

	m.mutex.Lock()
	delete(m.processes, channelID)
	m.mutex.Unlock()

	// Emitir evento de detención
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

	// Input
	args = append(args,
		"-re", // Leer a velocidad real
		"-i", config.InputPath,
	)

	// Codec de video - H.264 para SRT
	args = append(args,
		"-c:v", "libx264",
		"-preset", "fast",
		"-tune", "zerolatency", // Baja latencia para streaming
	)

	// Bitrate de video
	if config.VideoBitrate != "" {
		args = append(args, "-b:v", config.VideoBitrate)
	} else {
		args = append(args, "-b:v", "10M")
	}

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

	// Codec de audio - AAC para SRT
	args = append(args,
		"-c:a", "aac",
		"-ar", "48000",
		"-ac", "2",
	)

	// Bitrate de audio
	if config.AudioBitrate != "" {
		args = append(args, "-b:a", config.AudioBitrate)
	} else {
		args = append(args, "-b:a", "192k")
	}

	// Output SRT (modo listener - el servidor espera conexiones)
	srtPort := config.SRTPort
	if srtPort == 0 {
		srtPort = 9000 // Puerto por defecto
	}

	// Formato MPEG-TS para SRT
	args = append(args,
		"-f", "mpegts",
		fmt.Sprintf("srt://0.0.0.0:%d?mode=listener&latency=200000&pkt_size=1316", srtPort),
	)

	return args
}

// buildFFmpegArgsAlt construye argumentos alternativos (sin NDI nativo)
// Usa output a pipe/socket para integración con NDI SDK
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
	m.mutex.Unlock()
}

// parseProgress parsea la salida de FFmpeg para obtener progreso
func (m *Manager) parseProgress(channelID string, proc *ffmpegProcess) {
	scanner := bufio.NewScanner(proc.stderr)

	// Regex para parsear línea de progreso de FFmpeg
	frameRegex := regexp.MustCompile(`frame=\s*(\d+)`)
	fpsRegex := regexp.MustCompile(`fps=\s*([\d.]+)`)
	bitrateRegex := regexp.MustCompile(`bitrate=\s*([\d.]+\s*\w+/s)`)
	sizeRegex := regexp.MustCompile(`size=\s*(\d+)\s*\w+`)
	timeRegex := regexp.MustCompile(`time=\s*([\d:.]+)`)
	speedRegex := regexp.MustCompile(`speed=\s*([\d.]+x)`)

	for scanner.Scan() {
		line := scanner.Text()

		// Log para debugging
		log.Printf("[FFmpeg %s] %s", channelID, line)

		// Parsear progreso
		progress := Progress{}

		if matches := frameRegex.FindStringSubmatch(line); len(matches) > 1 {
			progress.Frame, _ = strconv.ParseInt(matches[1], 10, 64)
		}
		if matches := fpsRegex.FindStringSubmatch(line); len(matches) > 1 {
			progress.FPS, _ = strconv.ParseFloat(matches[1], 64)
		}
		if matches := bitrateRegex.FindStringSubmatch(line); len(matches) > 1 {
			progress.Bitrate = matches[1]
		}
		if matches := sizeRegex.FindStringSubmatch(line); len(matches) > 1 {
			progress.TotalSize, _ = strconv.ParseInt(matches[1], 10, 64)
		}
		if matches := timeRegex.FindStringSubmatch(line); len(matches) > 1 {
			progress.OutTime = matches[1]
		}
		if matches := speedRegex.FindStringSubmatch(line); len(matches) > 1 {
			progress.Speed = matches[1]
		}

		// Actualizar progreso si hay datos válidos
		if progress.Frame > 0 || progress.FPS > 0 {
			m.mutex.Lock()
			if p, exists := m.processes[channelID]; exists {
				p.progress = progress
			}
			m.mutex.Unlock()

			m.emitEvent(Event{
				Type:      EventProgress,
				ChannelID: channelID,
				Data: map[string]interface{}{
					"frame":   progress.Frame,
					"fps":     progress.FPS,
					"bitrate": progress.Bitrate,
					"time":    progress.OutTime,
					"speed":   progress.Speed,
				},
			})
		}

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

// HasNDISupport verifica si FFmpeg tiene soporte NDI
func HasNDISupport(ffmpegPath string) bool {
	formats, err := GetFFmpegFormats(ffmpegPath)
	if err != nil {
		return false
	}

	for _, format := range formats {
		if strings.Contains(format, "ndi") {
			return true
		}
	}

	return false
}
