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
		DefaultVideoBitrate: "10M",
		DefaultAudioBitrate: "192k",
		DefaultFrameRate:    30,
		TestPatternPath:     testPatternPath,
		SRTPrefix:           "SRT_SERVER_",
		SRTGroup:            "",
		DefaultVideoPath:    "",
		LogPath:             "",
		Theme:               "dark",
		Language:            "es",
		MaxLogLines:         1000,
	}
}

// GetConfigPath retorna la ruta del archivo de configuración
func GetConfigPath() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "servidor-stream", "config.json")
}

// Load carga la configuración desde archivo
func Load() (*Config, error) {
	configPath := GetConfigPath()

	// Crear directorio si no existe
	os.MkdirAll(filepath.Dir(configPath), 0755)

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

// SaveChannels guarda la configuración de canales
func SaveChannels(channels interface{}) error {
	configDir, _ := os.UserConfigDir()
	channelsPath := filepath.Join(configDir, "servidor-stream", "channels.json")

	data, err := json.MarshalIndent(channels, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(channelsPath, data, 0644)
}

// LoadChannels carga la configuración de canales
func LoadChannels() ([]byte, error) {
	configDir, _ := os.UserConfigDir()
	channelsPath := filepath.Join(configDir, "servidor-stream", "channels.json")

	return os.ReadFile(channelsPath)
}
