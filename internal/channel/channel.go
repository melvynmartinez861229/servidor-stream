package channel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status representa el estado de un canal
type Status string

const (
	StatusInactive Status = "inactive"
	StatusActive   Status = "active"
	StatusError    Status = "error"
	StatusStarting Status = "starting"
	StatusStopping Status = "stopping"
)

// Channel representa un canal de video SRT
type Channel struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	VideoPath     string    `json:"videoPath"`
	SRTStreamName string    `json:"srtStreamName"` // Nombre identificador del stream SRT
	SRTPort       int       `json:"srtPort"`       // Puerto SRT para este canal
	Resolution    string    `json:"resolution"`    // Resolución de salida (ej: "1920x1080")
	FrameRate     int       `json:"frameRate"`     // FPS de salida
	Status        Status    `json:"status"`
	CurrentFile   string    `json:"currentFile"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ErrorMessage  string    `json:"errorMessage,omitempty"`
	Stats         Stats     `json:"stats"`
}

// Stats contiene estadísticas del canal
type Stats struct {
	FramesProcessed int64         `json:"framesProcessed"`
	BytesSent       int64         `json:"bytesSent"`
	Uptime          time.Duration `json:"uptime"`
	LastError       string        `json:"lastError,omitempty"`
	ErrorCount      int           `json:"errorCount"`
}

// Manager gestiona los canales de video
type Manager struct {
	channels    map[string]*Channel
	mutex       sync.RWMutex
	persistPath string
}

// NewManager crea un nuevo gestor de canales
func NewManager() *Manager {
	// Determinar ruta de persistencia junto al ejecutable (portable)
	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	exeDir := filepath.Dir(exePath)
	persistPath := filepath.Join(exeDir, "channels.json")

	m := &Manager{
		channels:    make(map[string]*Channel),
		persistPath: persistPath,
	}

	// Cargar canales guardados
	m.loadFromDisk()

	return m
}

// getNextSRTPort calcula el siguiente puerto SRT disponible
func (m *Manager) getNextSRTPort() int {
	basePort := 9000
	maxPort := basePort

	for _, ch := range m.channels {
		if ch.SRTPort >= maxPort {
			maxPort = ch.SRTPort + 1
		}
	}

	if maxPort < basePort {
		return basePort
	}
	return maxPort
}

// Add agrega un nuevo canal (videoPath es opcional - Aximmetry lo envía dinámicamente)
func (m *Manager) Add(label, videoPath, srtStreamName string) (*Channel, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Validar parámetros
	if label == "" {
		return nil, errors.New("la etiqueta no puede estar vacía")
	}
	// videoPath ya no es obligatorio - Aximmetry lo envía vía WebSocket

	// Generar nombre de stream si no se proporciona
	if srtStreamName == "" {
		srtStreamName = "STREAM_" + label
	}

	// Verificar que el nombre de stream no esté en uso
	for _, ch := range m.channels {
		if ch.SRTStreamName == srtStreamName {
			return nil, errors.New("el nombre de stream ya está en uso")
		}
	}

	// Asignar puerto SRT único
	srtPort := m.getNextSRTPort()

	channel := &Channel{
		ID:            uuid.New().String(),
		Label:         label,
		VideoPath:     videoPath,
		SRTStreamName: srtStreamName,
		SRTPort:       srtPort,
		Resolution:    "1920x1080", // Valor por defecto
		FrameRate:     30,          // Valor por defecto
		Status:        StatusInactive,
		CurrentFile:   "", // Se llenará cuando Aximmetry solicite un video
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Stats:         Stats{},
	}

	m.channels[channel.ID] = channel

	// Persistir cambios a disco
	m.saveToDisk()

	return channel, nil
}

// Remove elimina un canal por ID
func (m *Manager) Remove(channelID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.channels[channelID]; !exists {
		return errors.New("canal no encontrado")
	}

	delete(m.channels, channelID)

	// Persistir cambios a disco
	m.saveToDisk()

	return nil
}

// Get obtiene un canal por ID
func (m *Manager) Get(channelID string) (*Channel, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return nil, errors.New("canal no encontrado")
	}

	return channel, nil
}

// GetBySRTName obtiene un canal por nombre SRT
func (m *Manager) GetBySRTName(srtName string) (*Channel, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, ch := range m.channels {
		if ch.SRTStreamName == srtName {
			return ch, nil
		}
	}

	return nil, errors.New("canal no encontrado")
}

// GetAll retorna todos los canales
func (m *Manager) GetAll() []Channel {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	channels := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, *ch)
	}

	return channels
}

// GetActive retorna los canales activos
func (m *Manager) GetActive() []Channel {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	channels := make([]Channel, 0)
	for _, ch := range m.channels {
		if ch.Status == StatusActive {
			channels = append(channels, *ch)
		}
	}

	return channels
}

// Update actualiza un canal existente
func (m *Manager) Update(channelID, label, videoPath, srtStreamName string) (*Channel, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return nil, errors.New("canal no encontrado")
	}

	// Verificar nombre SRT único
	if srtStreamName != channel.SRTStreamName {
		for _, ch := range m.channels {
			if ch.ID != channelID && ch.SRTStreamName == srtStreamName {
				return nil, errors.New("el nombre de stream SRT ya está en uso")
			}
		}
	}

	if label != "" {
		channel.Label = label
	}
	if videoPath != "" {
		channel.VideoPath = videoPath
	}
	if srtStreamName != "" {
		channel.SRTStreamName = srtStreamName
	}

	channel.UpdatedAt = time.Now()

	// Persistir cambios a disco
	m.saveToDisk()

	return channel, nil
}

// SetStatus establece el estado de un canal
func (m *Manager) SetStatus(channelID string, status Status) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return errors.New("canal no encontrado")
	}

	channel.Status = status
	channel.UpdatedAt = time.Now()

	return nil
}

// SetCurrentFile establece el archivo actual de un canal
func (m *Manager) SetCurrentFile(channelID, filePath string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return errors.New("canal no encontrado")
	}

	channel.CurrentFile = filePath
	channel.UpdatedAt = time.Now()

	return nil
}

// SetVideoSettings establece la resolución y FPS de un canal
func (m *Manager) SetVideoSettings(channelID, resolution string, frameRate int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return errors.New("canal no encontrado")
	}

	channel.Resolution = resolution
	channel.FrameRate = frameRate
	channel.UpdatedAt = time.Now()

	// Persistir cambios
	m.saveToDisk()

	return nil
}

// SetError establece un error en el canal
func (m *Manager) SetError(channelID, errorMessage string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return errors.New("canal no encontrado")
	}

	channel.Status = StatusError
	channel.ErrorMessage = errorMessage
	channel.Stats.LastError = errorMessage
	channel.Stats.ErrorCount++
	channel.UpdatedAt = time.Now()

	return nil
}

// UpdateStats actualiza las estadísticas de un canal
func (m *Manager) UpdateStats(channelID string, stats Stats) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return errors.New("canal no encontrado")
	}

	channel.Stats = stats

	return nil
}

// Count retorna el número total de canales
func (m *Manager) Count() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.channels)
}

// ActiveCount retorna el número de canales activos
func (m *Manager) ActiveCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	count := 0
	for _, ch := range m.channels {
		if ch.Status == StatusActive {
			count++
		}
	}

	return count
}

// saveToDisk guarda los canales a disco
func (m *Manager) saveToDisk() error {
	channels := make([]*Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}

	data, err := json.MarshalIndent(channels, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.persistPath, data, 0644)
}

// loadFromDisk carga los canales desde disco
func (m *Manager) loadFromDisk() error {
	data, err := os.ReadFile(m.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No hay archivo, es normal en primera ejecución
		}
		return err
	}

	var channels []*Channel
	if err := json.Unmarshal(data, &channels); err != nil {
		return err
	}

	for _, ch := range channels {
		// Resetear estado volátil al cargar
		ch.Status = StatusInactive
		ch.CurrentFile = ""
		ch.ErrorMessage = ""
		m.channels[ch.ID] = ch
	}

	return nil
}
