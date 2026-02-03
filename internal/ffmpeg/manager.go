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
	EventWarning  EventType = "warning"
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
	return m.startInternal(config, false)
}

// StartWithFallback inicia un proceso FFmpeg con fallback automático a libx264 si el encoder de hardware falla
func (m *Manager) StartWithFallback(config StreamConfig) error {
	return m.startInternal(config, true)
}

// startInternal implementación interna de Start con opción de fallback
func (m *Manager) startInternal(config StreamConfig, enableFallback bool) error {
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

	// Si se usa encoder de hardware y hay fallback habilitado, verificar primero
	originalEncoder := config.VideoEncoder
	isHardwareEncoder := originalEncoder == "h264_nvenc" || originalEncoder == "h264_qsv" || originalEncoder == "h264_amf"

	if isHardwareEncoder && enableFallback {
		// Probar si el encoder de hardware funciona
		if !m.testHardwareEncoder(config.InputPath, originalEncoder) {
			// Fallback a libx264
			config.VideoEncoder = "libx264"
			m.emitEvent(Event{
				Type:      EventWarning,
				ChannelID: config.ChannelID,
				Message:   fmt.Sprintf("Encoder %s no disponible (driver incompatible). Usando libx264 como fallback.", originalEncoder),
				Data: map[string]interface{}{
					"originalEncoder": originalEncoder,
					"fallbackEncoder": "libx264",
					"reason":          "hardware_encoder_unavailable",
				},
			})
			log.Printf("[FFmpeg] WARNING: %s no disponible, usando libx264 como fallback para canal %s", originalEncoder, config.ChannelID)
		}
	}

	// Construir argumentos de FFmpeg
	args := m.buildFFmpegArgs(config)

	// Log del comando completo para debug
	log.Printf("[FFmpeg] Comando: %s %s", m.ffmpegPath, strings.Join(args, " "))

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

	encoderUsed := config.VideoEncoder
	if encoderUsed == "" {
		encoderUsed = "libx264"
	}

	// Log del comando FFmpeg completo para debugging (solo primeros 500 caracteres)
	cmdString := fmt.Sprintf("%s %v", m.ffmpegPath, strings.Join(args, " "))
	if len(cmdString) > 500 {
		cmdString = cmdString[:500] + "..."
	}
	log.Printf("[FFmpeg %s] Comando: %s", config.ChannelID, cmdString)

	m.emitEvent(Event{
		Type:      EventStarted,
		ChannelID: config.ChannelID,
		Message:   fmt.Sprintf("FFmpeg iniciado: PID=%d, Puerto=%d, Encoder=%s, Resolución=%dx%d@%dfps", cmd.Process.Pid, srtPort, encoderUsed, config.Width, config.Height, config.FrameRate),
		Data: map[string]interface{}{
			"pid":        cmd.Process.Pid,
			"streamName": config.SRTStreamName,
			"inputPath":  config.InputPath,
			"srtPort":    srtPort,
			"srtUrl":     fmt.Sprintf("srt://IP_SERVIDOR:%d", srtPort),
			"encoder":    encoderUsed,
			"resolution": fmt.Sprintf("%dx%d", config.Width, config.Height),
			"frameRate":  config.FrameRate,
			"bitrate":    config.VideoBitrate,
		},
	})

	return nil
}

// testHardwareEncoder prueba si un encoder de hardware está disponible y funcional
func (m *Manager) testHardwareEncoder(inputPath string, encoder string) bool {
	// Crear un comando de prueba rápido (solo 1 frame)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-c:v", encoder,
		"-frames:v", "1",
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, m.ffmpegPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[FFmpeg] Test encoder %s falló: %v - %s", encoder, err, string(output))
		return false
	}

	log.Printf("[FFmpeg] Test encoder %s exitoso", encoder)
	return true
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

	// Input - optimizado para baja latencia
	args = append(args,
		"-re",                // Sincronización de tiempo real
		"-fflags", "+genpts", // Generar timestamps correctos
		"-fflags", "+nobuffer", // Sin buffering adicional
		"-avioflags", "direct", // I/O directo sin cache
		"-probesize", "32", // Probe mínimo para inicio rápido
		"-analyzeduration", "0", // No analizar duración para inicio instantáneo
		"-i", config.InputPath,
	)

	// === Encoder de Video ===
	args = append(args, "-c:v", encoder)

	// Configuración específica por encoder
	switch encoder {
	case "h264_nvenc":
		// NVIDIA NVENC - Aceleración por hardware (RTX 3060 Ti y similares)
		// Configuración básica y universal compatible con todas las versiones de FFmpeg

		// GOP size para NVENC
		gopSize := config.GopSize
		if gopSize <= 0 {
			gopSize = 60 // 2 segundos a 30fps
		}

		// Configuración mínima y robusta para NVENC
		args = append(args,
			"-g", strconv.Itoa(gopSize), // GOP size
			"-bf", "0", // Sin B-frames para baja latencia
		)

	case "h264_qsv":
		// Intel QuickSync
		preset := config.EncoderPreset
		if preset == "" {
			preset = "veryfast"
		}
		gopSize := config.GopSize
		if gopSize <= 0 {
			gopSize = 60
		}
		args = append(args,
			"-preset", preset,
			"-look_ahead", "0", // Deshabilitar lookahead para baja latencia
			"-g", strconv.Itoa(gopSize),
			"-bf", "0",
		)
		if config.EncoderProfile != "" {
			args = append(args, "-profile:v", config.EncoderProfile)
		}

	case "h264_amf":
		// AMD AMF
		gopSize := config.GopSize
		if gopSize <= 0 {
			gopSize = 60
		}
		args = append(args,
			"-quality", "speed",
			"-rc", "cbr",
			"-g", strconv.Itoa(gopSize),
			"-bf", "0",
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

		// GOP y B-Frames para libx264 - ultra baja latencia
		gopSize := config.GopSize
		if gopSize <= 0 {
			gopSize = 15 // 0.5 segundos a 30fps - keyframes frecuentes para baja latencia
		}
		args = append(args,
			"-g", strconv.Itoa(gopSize),
			"-keyint_min", strconv.Itoa(gopSize),
			"-sc_threshold", "0", // Deshabilitar detección de cambio de escena
		)

		bframes := config.BFrames
		args = append(args, "-bf", strconv.Itoa(bframes))
	}

	// === Control de Bitrate ===
	videoBitrate := config.VideoBitrate
	if videoBitrate == "" {
		videoBitrate = "5M"
	}
	args = append(args, "-b:v", videoBitrate)

	// maxrate y bufsize solo para encoders que los soportan bien
	if encoder != "h264_nvenc" {
		maxBitrate := config.MaxBitrate
		if maxBitrate == "" {
			maxBitrate = videoBitrate // CBR: maxrate = bitrate
		}
		bufSize := config.BufferSize
		if bufSize == "" {
			bufSize = videoBitrate // CBR: bufsize = bitrate
		}
		args = append(args, "-maxrate", maxBitrate, "-bufsize", bufSize)
	}

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

	// Parámetros SRT optimizados para baja latencia
	srtLatency := config.SRTLatency
	if srtLatency <= 0 {
		srtLatency = 120 // 120ms por defecto (ultra baja latencia en LAN)
	}
	srtLatencyUs := srtLatency * 1000 // Convertir a microsegundos

	srtRecvBuf := config.SRTRecvBuffer
	if srtRecvBuf <= 0 {
		srtRecvBuf = 2097152 // 2MB por defecto (reducido para baja latencia)
	}

	srtSendBuf := config.SRTSendBuffer
	if srtSendBuf <= 0 {
		srtSendBuf = 2097152 // 2MB por defecto (reducido para baja latencia)
	}

	srtOverhead := config.SRTOverheadBW
	if srtOverhead <= 0 {
		srtOverhead = 25 // 25% por defecto
	}

	// Calcular muxrate basado en bitrate - ajustado para baja latencia
	muxrate := "6M" // Reducido para menor buffering

	// Construir URL SRT con parámetros optimizados para ultra baja latencia
	srtURL := fmt.Sprintf(
		"srt://%s:%d?mode=listener&latency=%d&pkt_size=1316&rcvbuf=%d&sndbuf=%d&maxbw=-1&oheadbw=%d&listen_timeout=-1&tlpktdrop=1&nakreport=1",
		srtHost, srtPort, srtLatencyUs, srtRecvBuf, srtSendBuf, srtOverhead,
	)

	args = append(args,
		"-f", "mpegts",
		"-mpegts_copyts", "1",
		"-mpegts_flags", "latm", // Modo de baja latencia para MPEG-TS
		"-flush_packets", "1", // Flush inmediato de paquetes
		"-muxrate", muxrate,
		"-pcr_period", "20", // PCR cada 20ms para sincronización precisa
		"-muxdelay", "0.1", // Delay mínimo del muxer (100ms)
		"-max_delay", "100000", // Máximo delay 100ms
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
	lastProgressLog := time.Now()
	progressLogInterval := 30 * time.Second // Log de progreso cada 30 segundos
	streamingStarted := false

	for scanner.Scan() {
		line := scanner.Text()
		lineLower := strings.ToLower(line)

		// Detectar cuando un cliente SRT se conecta
		if strings.Contains(lineLower, "srt: accepted connection") || strings.Contains(lineLower, "srt: listener accepted") {
			log.Printf("[FFmpeg %s] ✓ Cliente SRT conectado", channelID)
			m.emitEvent(Event{
				Type:      EventProgress,
				ChannelID: channelID,
				Message:   "Cliente SRT conectado - streaming activo",
			})
			streamingStarted = true
		}

		// Detectar progreso de frames (indica que está strimeando)
		if strings.Contains(line, "frame=") && strings.Contains(line, "fps=") {
			if !streamingStarted {
				log.Printf("[FFmpeg %s] ✓ Streaming iniciado - generando frames", channelID)
				streamingStarted = true
			}

			// Log periódico (cada 30s) para confirmar que sigue strimeando
			if time.Since(lastProgressLog) >= progressLogInterval {
				// Extraer info básica del progreso
				progressInfo := line
				if len(progressInfo) > 150 {
					progressInfo = progressInfo[:150] + "..."
				}
				log.Printf("[FFmpeg %s] → Streaming: %s", channelID, progressInfo)
				lastProgressLog = time.Now()

				// Emitir evento de progreso (sin llenar memoria)
				m.emitEvent(Event{
					Type:      EventProgress,
					ChannelID: channelID,
					Message:   "Streaming activo",
					Data: map[string]interface{}{
						"uptime": time.Since(proc.startTime).String(),
					},
				})
			}
		}

		// Log completo solo para errores y warnings importantes
		if strings.Contains(lineLower, "error") && !strings.Contains(lineLower, "no error") {
			log.Printf("[FFmpeg %s] ✗ ERROR: %s", channelID, line)
			m.emitEvent(Event{
				Type:      EventError,
				ChannelID: channelID,
				Message:   line,
			})
		} else if strings.Contains(lineLower, "warning") {
			log.Printf("[FFmpeg %s] ⚠ WARNING: %s", channelID, line)
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
