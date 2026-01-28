package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config configuración de la aplicación
type Config struct {
	// Servidor
	WebSocketPort int    `json:"webSocketPort"`
	FFmpegPath    string `json:"ffmpegPath"`
	AutoRestart   bool   `json:"autoRestart"`

	// Video por defecto
	DefaultVideoBitrate string `json:"defaultVideoBitrate"`
	DefaultAudioBitrate string `json:"defaultAudioBitrate"`
	DefaultFrameRate    int    `json:"defaultFrameRate"`

	// Patrón de prueba
	TestPatternPath string `json:"testPatternPath"` // Ruta al video patrón para pruebas

	// SRT
	SRTPrefix string `json:"srtPrefix"`
	SRTGroup  string `json:"srtGroup"`

	// Rutas
	DefaultVideoPath string `json:"defaultVideoPath"`
	LogPath          string `json:"logPath"`

	// UI
	Theme       string `json:"theme"`
	Language    string `json:"language"`
	MaxLogLines int    `json:"maxLogLines"`

	// === Configuración Avanzada de Streaming ===

	// Encoding
	VideoEncoder   string `json:"videoEncoder"`   // libx264, h264_nvenc, h264_qsv
	EncoderPreset  string `json:"encoderPreset"`  // ultrafast, veryfast, fast, medium
	EncoderProfile string `json:"encoderProfile"` // baseline, main, high
	EncoderTune    string `json:"encoderTune"`    // zerolatency, film, animation
	GopSize        int    `json:"gopSize"`        // Keyframe interval (frames)
	BFrames        int    `json:"bFrames"`        // B-frames (0 para baja latencia)

	// Bitrate Control
	BitrateMode string `json:"bitrateMode"` // cbr, vbr
	MaxBitrate  string `json:"maxBitrate"`  // Máximo bitrate (para VBR)
	BufferSize  string `json:"bufferSize"`  // Tamaño del buffer de rate control
	CRF         int    `json:"crf"`         // Calidad constante (0-51, solo VBR)

	// SRT Avanzado
	SRTLatency      int `json:"srtLatency"`      // Latencia SRT en ms
	SRTRecvBuffer   int `json:"srtRecvBuffer"`   // Buffer de recepción en bytes
	SRTSendBuffer   int `json:"srtSendBuffer"`   // Buffer de envío en bytes
	SRTOverheadBW   int `json:"srtOverheadBW"`   // Overhead bandwidth %
	SRTPeerIdleTime int `json:"srtPeerIdleTime"` // Timeout de peer idle en ms
}

// GetExecutablePath retorna la ruta del ejecutable
func GetExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// GetLocalFFmpegPath retorna la ruta de FFmpeg local (carpeta ffmpeg junto al exe)
func GetLocalFFmpegPath() string {
	exeDir := GetExecutablePath()
	if exeDir == "" {
		return ""
	}

	// Buscar ffmpeg.exe en la carpeta ffmpeg junto al ejecutable
	ffmpegPath := filepath.Join(exeDir, "ffmpeg", "ffmpeg.exe")
	if _, err := os.Stat(ffmpegPath); err == nil {
		return ffmpegPath
	}

	// También buscar en carpeta bin dentro de ffmpeg
	ffmpegPath = filepath.Join(exeDir, "ffmpeg", "bin", "ffmpeg.exe")
	if _, err := os.Stat(ffmpegPath); err == nil {
		return ffmpegPath
	}

	return ""
}

// GetLocalTestPatternPath retorna la ruta del video patrón junto al ejecutable
func GetLocalTestPatternPath() string {
	exeDir := GetExecutablePath()
	if exeDir == "" {
		return ""
	}

	// Buscar patron.mp4 junto al ejecutable
	patternPath := filepath.Join(exeDir, "patron.mp4")
	if _, err := os.Stat(patternPath); err == nil {
		return patternPath
	}

	// Buscar en carpeta assets
	patternPath = filepath.Join(exeDir, "assets", "patron.mp4")
	if _, err := os.Stat(patternPath); err == nil {
		return patternPath
	}

	return ""
}

// Default retorna la configuración por defecto
func Default() *Config {
	// Intentar usar FFmpeg local primero
	ffmpegPath := GetLocalFFmpegPath()
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg" // Fallback al PATH del sistema
	}

	// Buscar patrón de prueba junto al ejecutable
	testPatternPath := GetLocalTestPatternPath()

	return &Config{
		WebSocketPort:       8765,
		FFmpegPath:          ffmpegPath,
		AutoRestart:         true,
		DefaultVideoBitrate: "5M",
		DefaultAudioBitrate: "192k",
		DefaultFrameRate:    25,
		TestPatternPath:     testPatternPath,
		SRTPrefix:           "SRT_SERVER_",
		SRTGroup:            "",
		DefaultVideoPath:    "",
		LogPath:             "",
		Theme:               "dark",
		Language:            "es",
		MaxLogLines:         1000,
		// Encoding defaults optimizados para estabilidad
		VideoEncoder:   "libx264",
		EncoderPreset:  "veryfast",
		EncoderProfile: "main",
		EncoderTune:    "zerolatency",
		GopSize:        50, // 2 segundos a 25fps
		BFrames:        0,  // Sin B-frames para baja latencia
		// Bitrate Control
		BitrateMode: "cbr",
		MaxBitrate:  "5M",
		BufferSize:  "5M",
		CRF:         23,
		// SRT optimizado para estabilidad
		SRTLatency:      500,     // 500ms de latencia
		SRTRecvBuffer:   8388608, // 8MB
		SRTSendBuffer:   8388608, // 8MB
		SRTOverheadBW:   25,      // 25% overhead
		SRTPeerIdleTime: 5000,    // 5 segundos
	}
}

// GetConfigPath retorna la ruta del archivo de configuración (junto al ejecutable)
func GetConfigPath() string {
	exePath, err := os.Executable()
	if err != nil {
		// Fallback al directorio actual
		return "config.json"
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, "config.json")
}

// Load carga la configuración desde archivo
func Load() (*Config, error) {
	configPath := GetConfigPath()

	// Leer archivo
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Crear configuración por defecto
			cfg := Default()
			Save(cfg)
			return cfg, nil
		}
		return nil, err
	}

	// Parsear JSON
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save guarda la configuración a archivo
func Save(cfg *Config) error {
	configPath := GetConfigPath()

	// Crear directorio si no existe
	os.MkdirAll(filepath.Dir(configPath), 0755)

	// Serializar a JSON
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// Escribir archivo
	return os.WriteFile(configPath, data, 0644)
}

// GetChannelsPath retorna la ruta del archivo de canales (junto al ejecutable)
func GetChannelsPath() string {
	exePath, err := os.Executable()
	if err != nil {
		// Fallback al directorio actual
		return "channels.json"
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, "channels.json")
}

// SaveChannels guarda la configuración de canales
func SaveChannels(channels interface{}) error {
	channelsPath := GetChannelsPath()

	// Crear directorio si no existe
	os.MkdirAll(filepath.Dir(channelsPath), 0755)

	data, err := json.MarshalIndent(channels, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(channelsPath, data, 0644)
}

// LoadChannels carga la configuración de canales
func LoadChannels() ([]byte, error) {
	channelsPath := GetChannelsPath()

	return os.ReadFile(channelsPath)
}
