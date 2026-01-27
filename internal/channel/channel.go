package channel

import (
	"errors"
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
	ID                string    `json:"id"`
	Label             string    `json:"label"`
	VideoPath         string    `json:"videoPath"`
	NDIStreamName     string    `json:"ndiStreamName"` // Se mantiene por compatibilidad (nombre del stream)
	SRTPort           int       `json:"srtPort"`       // Puerto SRT para este canal
	Status            Status    `json:"status"`
	PreviewEnabled    bool      `json:"previewEnabled"`
	CurrentFile       string    `json:"currentFile"`
	PreviewBase64     string    `json:"previewBase64,omitempty"`
	LastPreviewUpdate time.Time `json:"lastPreviewUpdate"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	ErrorMessage      string    `json:"errorMessage,omitempty"`
	Stats             Stats     `json:"stats"`
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
	channels map[string]*Channel
	mutex    sync.RWMutex
}

// NewManager crea un nuevo gestor de canales
func NewManager() *Manager {
	return &Manager{
		channels: make(map[string]*Channel),
	}
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
func (m *Manager) Add(label, videoPath, ndiStreamName string) (*Channel, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Validar parámetros
	if label == "" {
		return nil, errors.New("la etiqueta no puede estar vacía")
	}
	// videoPath ya no es obligatorio - Aximmetry lo envía vía WebSocket

	// Generar nombre de stream si no se proporciona
	if ndiStreamName == "" {
		ndiStreamName = "STREAM_" + label
	}

	// Verificar que el nombre de stream no esté en uso
	for _, ch := range m.channels {
		if ch.NDIStreamName == ndiStreamName {
			return nil, errors.New("el nombre de stream ya está en uso")
		}
	}

	// Asignar puerto SRT único
	srtPort := m.getNextSRTPort()

	channel := &Channel{
		ID:             uuid.New().String(),
		Label:          label,
		VideoPath:      videoPath,
		NDIStreamName:  ndiStreamName,
		SRTPort:        srtPort,
		Status:         StatusInactive,
		PreviewEnabled: true,
		CurrentFile:    "", // Se llenará cuando Aximmetry solicite un video
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Stats:          Stats{},
	}

	m.channels[channel.ID] = channel

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

// GetByNDIName obtiene un canal por nombre NDI
func (m *Manager) GetByNDIName(ndiName string) (*Channel, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, ch := range m.channels {
		if ch.NDIStreamName == ndiName {
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
func (m *Manager) Update(channelID, label, videoPath, ndiStreamName string) (*Channel, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return nil, errors.New("canal no encontrado")
	}

	// Verificar nombre NDI único
	if ndiStreamName != channel.NDIStreamName {
		for _, ch := range m.channels {
			if ch.ID != channelID && ch.NDIStreamName == ndiStreamName {
				return nil, errors.New("el nombre de stream NDI ya está en uso")
			}
		}
	}

	if label != "" {
		channel.Label = label
	}
	if videoPath != "" {
		channel.VideoPath = videoPath
	}
	if ndiStreamName != "" {
		channel.NDIStreamName = ndiStreamName
	}

	channel.UpdatedAt = time.Now()

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

// SetPreviewEnabled habilita o deshabilita la previsualización
func (m *Manager) SetPreviewEnabled(channelID string, enabled bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return errors.New("canal no encontrado")
	}

	channel.PreviewEnabled = enabled
	channel.UpdatedAt = time.Now()

	return nil
}

// SetPreview actualiza la previsualización de un canal
func (m *Manager) SetPreview(channelID, previewBase64 string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	channel, exists := m.channels[channelID]
	if !exists {
		return errors.New("canal no encontrado")
	}

	channel.PreviewBase64 = previewBase64
	channel.LastPreviewUpdate = time.Now()

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
