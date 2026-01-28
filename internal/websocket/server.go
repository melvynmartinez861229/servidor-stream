package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Message representa un mensaje WebSocket
type Message struct {
	Action     string                 `json:"action"`
	ClientID   string                 `json:"clientId,omitempty"`
	ChannelID  string                 `json:"channelId,omitempty"`
	FilePath   string                 `json:"filePath,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// Response representa una respuesta WebSocket
type Response struct {
	Success bool        `json:"success"`
	Action  string      `json:"action"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ClientInfo información de un cliente conectado
type ClientInfo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	ConnectedAt   time.Time `json:"connectedAt"`
	LastMessageAt time.Time `json:"lastMessageAt"`
	MessageCount  int       `json:"messageCount"`
	RemoteAddr    string    `json:"remoteAddr"`
}

// Client representa un cliente WebSocket conectado
type Client struct {
	ID            string
	Name          string
	conn          *websocket.Conn
	send          chan []byte
	server        *Server
	connectedAt   time.Time
	lastMessageAt time.Time
	messageCount  int
	remoteAddr    string
}

// Server servidor WebSocket
type Server struct {
	port               int
	clients            map[string]*Client
	mutex              sync.RWMutex
	upgrader           websocket.Upgrader
	messageHandler     func(clientID string, message []byte) []byte
	onClientConnect    func(client ClientInfo)
	onClientDisconnect func(clientID string)
	httpServer         *http.Server
}

// NewServer crea un nuevo servidor WebSocket
func NewServer(port int, handler func(clientID string, message []byte) []byte) *Server {
	return &Server{
		port:    port,
		clients: make(map[string]*Client),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Permitir todas las conexiones (ajustar en producción)
			},
		},
		messageHandler: handler,
	}
}

// Start inicia el servidor WebSocket
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleConnection)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/channels", s.handleChannelsAPI)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	log.Printf("Servidor WebSocket iniciando en puerto %d", s.port)

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// Stop detiene el servidor WebSocket
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}

	// Cerrar todas las conexiones de clientes
	s.mutex.Lock()
	for _, client := range s.clients {
		client.conn.Close()
	}
	s.clients = make(map[string]*Client)
	s.mutex.Unlock()

	log.Println("Servidor WebSocket detenido")
}

// handleConnection maneja nuevas conexiones WebSocket
func (s *Server) handleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %v", err)
		return
	}

	clientID := uuid.New().String()
	clientName := r.URL.Query().Get("name")
	if clientName == "" {
		clientName = "Aximmetry_" + clientID[:8]
	}

	client := &Client{
		ID:          clientID,
		Name:        clientName,
		conn:        conn,
		send:        make(chan []byte, 256),
		server:      s,
		connectedAt: time.Now(),
		remoteAddr:  r.RemoteAddr,
	}

	s.registerClient(client)

	// Enviar mensaje de bienvenida
	welcome := Response{
		Success: true,
		Action:  "connected",
		Message: "Conectado al servidor SRT Stream",
		Data: map[string]interface{}{
			"clientId": clientID,
			"name":     clientName,
		},
	}
	welcomeBytes, _ := json.Marshal(welcome)
	client.send <- welcomeBytes

	// Iniciar goroutines para lectura y escritura
	go client.writePump()
	go client.readPump()

	log.Printf("Cliente conectado: %s (%s) desde %s", clientName, clientID, r.RemoteAddr)
}

// handleHealth endpoint de salud
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"clients": len(s.clients),
		"time":    time.Now().Format(time.RFC3339),
	})
}

// handleChannelsAPI endpoint REST para canales
func (s *Server) handleChannelsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		return
	}

	// Este endpoint será manejado por la aplicación principal
	response := s.messageHandler("api", []byte(`{"action":"list_channels"}`))
	w.Write(response)
}

// registerClient registra un nuevo cliente
func (s *Server) registerClient(client *Client) {
	s.mutex.Lock()
	s.clients[client.ID] = client
	s.mutex.Unlock()

	// Notificar conexión
	if s.onClientConnect != nil {
		s.onClientConnect(ClientInfo{
			ID:            client.ID,
			Name:          client.Name,
			ConnectedAt:   client.connectedAt,
			LastMessageAt: client.lastMessageAt,
			MessageCount:  client.messageCount,
			RemoteAddr:    client.remoteAddr,
		})
	}
}

// unregisterClient elimina un cliente
func (s *Server) unregisterClient(client *Client) {
	clientID := client.ID

	s.mutex.Lock()
	if _, ok := s.clients[clientID]; ok {
		delete(s.clients, clientID)
		close(client.send)
	}
	s.mutex.Unlock()

	log.Printf("Cliente desconectado: %s (%s)", client.Name, clientID)

	// Notificar desconexión
	if s.onClientDisconnect != nil {
		s.onClientDisconnect(clientID)
	}
}

// SetClientCallbacks establece los callbacks para eventos de clientes
func (s *Server) SetClientCallbacks(onConnect func(ClientInfo), onDisconnect func(string)) {
	s.onClientConnect = onConnect
	s.onClientDisconnect = onDisconnect
}

// GetClients retorna información de los clientes conectados
func (s *Server) GetClients() []ClientInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	clients := make([]ClientInfo, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, ClientInfo{
			ID:            c.ID,
			Name:          c.Name,
			ConnectedAt:   c.connectedAt,
			LastMessageAt: c.lastMessageAt,
			MessageCount:  c.messageCount,
			RemoteAddr:    c.remoteAddr,
		})
	}

	return clients
}

// Broadcast envía un mensaje a todos los clientes
func (s *Server) Broadcast(message []byte) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, client := range s.clients {
		select {
		case client.send <- message:
		default:
			// Canal lleno, cliente lento
		}
	}
}

// SendToClient envía un mensaje a un cliente específico
func (s *Server) SendToClient(clientID string, message []byte) error {
	s.mutex.RLock()
	client, ok := s.clients[clientID]
	s.mutex.RUnlock()

	if !ok {
		return fmt.Errorf("cliente no encontrado: %s", clientID)
	}

	select {
	case client.send <- message:
		return nil
	default:
		return fmt.Errorf("buffer del cliente lleno")
	}
}

// readPump lee mensajes del cliente
func (c *Client) readPump() {
	defer func() {
		c.server.unregisterClient(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024) // 512KB max message size
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error leyendo mensaje: %v", err)
			}
			break
		}

		c.lastMessageAt = time.Now()
		c.messageCount++

		// Procesar mensaje y obtener respuesta
		response := c.server.messageHandler(c.ID, message)
		if response != nil {
			c.send <- response
		}
	}
}

// writePump escribe mensajes al cliente
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Agregar mensajes en cola al mismo write
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ErrorResponse crea una respuesta de error
func ErrorResponse(action, errorMessage string) []byte {
	response := Response{
		Success: false,
		Action:  action,
		Error:   errorMessage,
	}
	bytes, _ := json.Marshal(response)
	return bytes
}

// SuccessResponse crea una respuesta exitosa
func SuccessResponse(action string, data interface{}) []byte {
	response := Response{
		Success: true,
		Action:  action,
		Data:    data,
	}
	bytes, _ := json.Marshal(response)
	return bytes
}
