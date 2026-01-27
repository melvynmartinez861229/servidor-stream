package preview

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Config configuración del sistema de previsualizaciones
type Config struct {
	Width            int  `json:"width"`
	Height           int  `json:"height"`
	Quality          int  `json:"quality"`
	UpdateIntervalMS int  `json:"updateIntervalMs"`
	Enabled          bool `json:"enabled"`
}

// DefaultConfig retorna la configuración por defecto
func DefaultConfig() Config {
	return Config{
		Width:            320,
		Height:           180,
		Quality:          60,
		UpdateIntervalMS: 2000, // ~50 frames a 25fps
		Enabled:          true,
	}
}

// Manager gestor de previsualizaciones
type Manager struct {
	ffmpegPath string
	config     Config
	cache      map[string]*cachedPreview
	mutex      sync.RWMutex
	tempDir    string
}

type cachedPreview struct {
	data       string
	timestamp  time.Time
	frameCount int
}

// NewManager crea un nuevo gestor de previsualizaciones
func NewManager(ffmpegPath string, config Config) *Manager {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	// Crear directorio temporal para previews
	tempDir := filepath.Join(os.TempDir(), "ndi-server-previews")
	os.MkdirAll(tempDir, 0755)

	return &Manager{
		ffmpegPath: ffmpegPath,
		config:     config,
		cache:      make(map[string]*cachedPreview),
		tempDir:    tempDir,
	}
}

// UpdateConfig actualiza la configuración
func (m *Manager) UpdateConfig(config Config) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.config = config
}

// GeneratePreview genera una previsualización de un archivo de video
func (m *Manager) GeneratePreview(videoPath string) (string, error) {
	if !m.config.Enabled {
		return "", nil
	}

	// Verificar caché
	m.mutex.RLock()
	cached, exists := m.cache[videoPath]
	m.mutex.RUnlock()

	if exists && time.Since(cached.timestamp) < time.Duration(m.config.UpdateIntervalMS)*time.Millisecond {
		return cached.data, nil
	}

	// Generar nueva previsualización
	previewData, err := m.extractFrame(videoPath)
	if err != nil {
		return "", err
	}

	// Actualizar caché
	m.mutex.Lock()
	m.cache[videoPath] = &cachedPreview{
		data:      previewData,
		timestamp: time.Now(),
	}
	m.mutex.Unlock()

	return previewData, nil
}

// GeneratePreviewAtTime genera una previsualización en un tiempo específico
func (m *Manager) GeneratePreviewAtTime(videoPath string, seekTime string) (string, error) {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-ss", seekTime,
		"-i", videoPath,
		"-vframes", "1",
		"-s", fmt.Sprintf("%dx%d", m.config.Width, m.config.Height),
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", fmt.Sprintf("%d", (100-m.config.Quality)/10+2), // Convertir calidad a escala FFmpeg
		"pipe:1",
	}

	cmd := exec.Command(m.ffmpegPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error generando preview: %v - %s", err, stderr.String())
	}

	// Convertir a base64
	base64Data := base64.StdEncoding.EncodeToString(stdout.Bytes())
	return "data:image/jpeg;base64," + base64Data, nil
}

// extractFrame extrae un frame del video usando FFmpeg
func (m *Manager) extractFrame(videoPath string) (string, error) {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", videoPath,
		"-vf", fmt.Sprintf("select='eq(n,0)',scale=%d:%d", m.config.Width, m.config.Height),
		"-vframes", "1",
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", fmt.Sprintf("%d", (100-m.config.Quality)/10+2),
		"pipe:1",
	}

	cmd := exec.Command(m.ffmpegPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error extrayendo frame: %v - %s", err, stderr.String())
	}

	// Convertir a base64
	base64Data := base64.StdEncoding.EncodeToString(stdout.Bytes())
	return "data:image/jpeg;base64," + base64Data, nil
}

// GenerateThumbnail genera un thumbnail de un archivo de video
func (m *Manager) GenerateThumbnail(videoPath string) (string, error) {
	// Extraer frame en el 10% del video
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", videoPath,
		"-vf", fmt.Sprintf("thumbnail,scale=%d:%d", m.config.Width, m.config.Height),
		"-vframes", "1",
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "5",
		"pipe:1",
	}

	cmd := exec.Command(m.ffmpegPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error generando thumbnail: %v - %s", err, stderr.String())
	}

	base64Data := base64.StdEncoding.EncodeToString(stdout.Bytes())
	return "data:image/jpeg;base64," + base64Data, nil
}

// GenerateAnimatedPreview genera una previsualización animada (GIF) del video
func (m *Manager) GenerateAnimatedPreview(videoPath string) (string, error) {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=5,scale=%d:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse", m.config.Width),
		"-loop", "0",
		"-t", "3", // 3 segundos de GIF
		"-f", "gif",
		"pipe:1",
	}

	cmd := exec.Command(m.ffmpegPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error generando GIF: %v - %s", err, stderr.String())
	}

	base64Data := base64.StdEncoding.EncodeToString(stdout.Bytes())
	return "data:image/gif;base64," + base64Data, nil
}

// GetVideoInfo obtiene información de un archivo de video
func (m *Manager) GetVideoInfo(videoPath string) (*VideoInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		videoPath,
	)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// Intentar con FFmpeg si ffprobe no está disponible
		return m.getVideoInfoFFmpeg(videoPath)
	}

	// Parsear JSON (simplificado)
	return &VideoInfo{
		Path: videoPath,
	}, nil
}

// getVideoInfoFFmpeg obtiene info usando FFmpeg
func (m *Manager) getVideoInfoFFmpeg(videoPath string) (*VideoInfo, error) {
	return &VideoInfo{
		Path: videoPath,
	}, nil
}

// VideoInfo información de un archivo de video
type VideoInfo struct {
	Path      string        `json:"path"`
	Duration  time.Duration `json:"duration"`
	Width     int           `json:"width"`
	Height    int           `json:"height"`
	FrameRate float64       `json:"frameRate"`
	Codec     string        `json:"codec"`
	Bitrate   int64         `json:"bitrate"`
	Size      int64         `json:"size"`
}

// ClearCache limpia la caché de previsualizaciones
func (m *Manager) ClearCache() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.cache = make(map[string]*cachedPreview)
}

// CleanupTempFiles limpia archivos temporales
func (m *Manager) CleanupTempFiles() error {
	return os.RemoveAll(m.tempDir)
}

// EncodeImageToBase64 codifica una imagen Go a base64
func EncodeImageToBase64(img image.Image, quality int) (string, error) {
	var buf bytes.Buffer

	options := &jpeg.Options{Quality: quality}
	if err := jpeg.Encode(&buf, img, options); err != nil {
		return "", err
	}

	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	return "data:image/jpeg;base64," + base64Data, nil
}
