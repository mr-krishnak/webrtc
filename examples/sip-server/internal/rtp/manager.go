// Package rtp provides RTP session management
package rtp

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/examples/sip-server/internal/config"
)

// Manager manages RTP sessions
type Manager struct {
	config       *config.Config
	logger       logging.LoggerFactory
	sessions     map[string]*Session
	sessionMutex sync.RWMutex
	portPool     *PortPool
}

// Session represents an RTP session
type Session struct {
	ID           string
	LocalAddr    *net.UDPAddr
	RemoteAddr   net.Addr
	LocalConn    *net.UDPConn
	StartTime    time.Time
	PacketsSent  uint64
	BytesSent    uint64
	PacketsRecv  uint64
	BytesRecv    uint64
	mutex        sync.RWMutex
}

// PortPool manages dynamic port allocation
type PortPool struct {
	minPort     int
	maxPort     int
	usedPorts   map[int]bool
	mutex       sync.Mutex
}

// NewManager creates a new RTP manager
func NewManager(cfg *config.Config, logger logging.LoggerFactory) *Manager {
	return &Manager{
		config:   cfg,
		logger:   logger,
		sessions: make(map[string]*Session),
		portPool: NewPortPool(cfg.Server.RTPPortRange.Min, cfg.Server.RTPPortRange.Max),
	}
}

// NewPortPool creates a new port pool
func NewPortPool(minPort, maxPort int) *PortPool {
	return &PortPool{
		minPort:   minPort,
		maxPort:   maxPort,
		usedPorts: make(map[int]bool),
	}
}

// AllocatePort allocates an available port from the pool
func (pp *PortPool) AllocatePort() (int, error) {
	pp.mutex.Lock()
	defer pp.mutex.Unlock()

	for port := pp.minPort; port <= pp.maxPort; port++ {
		if !pp.usedPorts[port] {
			pp.usedPorts[port] = true
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", pp.minPort, pp.maxPort)
}

// ReleasePort releases a port back to the pool
func (pp *PortPool) ReleasePort(port int) {
	pp.mutex.Lock()
	defer pp.mutex.Unlock()
	delete(pp.usedPorts, port)
}

// GetUsedPortsCount returns the number of used ports
func (pp *PortPool) GetUsedPortsCount() int {
	pp.mutex.Lock()
	defer pp.mutex.Unlock()
	return len(pp.usedPorts)
}

// CreateSession creates a new RTP session
func (m *Manager) CreateSession(sessionID string, remoteAddr net.Addr) (*Session, error) {
	log := m.logger.NewLogger("rtp-manager")

	// Allocate a local port
	localPort, err := m.portPool.AllocatePort()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate RTP port: %w", err)
	}

	// Create UDP address
	localAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", m.config.Server.BindAddress, localPort))
	if err != nil {
		m.portPool.ReleasePort(localPort)
		return nil, fmt.Errorf("failed to resolve local UDP address: %w", err)
	}

	// Create UDP connection
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		m.portPool.ReleasePort(localPort)
		return nil, fmt.Errorf("failed to create UDP connection: %w", err)
	}

	session := &Session{
		ID:         sessionID,
		LocalAddr:  localAddr,
		RemoteAddr: remoteAddr,
		LocalConn:  conn,
		StartTime:  time.Now(),
	}

	// Store session
	m.sessionMutex.Lock()
	m.sessions[sessionID] = session
	m.sessionMutex.Unlock()

	log.Infof("Created RTP session: %s on port %d", sessionID, localPort)

	// Start packet handling
	go m.handlePackets(session)

	return session, nil
}

// CloseSession closes an RTP session
func (m *Manager) CloseSession(sessionID string) error {
	log := m.logger.NewLogger("rtp-manager")

	m.sessionMutex.Lock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.sessionMutex.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	delete(m.sessions, sessionID)
	m.sessionMutex.Unlock()

	// Close UDP connection
	if err := session.LocalConn.Close(); err != nil {
		log.Errorf("Failed to close UDP connection for session %s: %v", sessionID, err)
	}

	// Release port
	m.portPool.ReleasePort(session.LocalAddr.Port)

	log.Infof("Closed RTP session: %s", sessionID)
	return nil
}

// GetSession returns an RTP session by ID
func (m *Manager) GetSession(sessionID string) (*Session, bool) {
	m.sessionMutex.RLock()
	defer m.sessionMutex.RUnlock()
	session, exists := m.sessions[sessionID]
	return session, exists
}

// GetSessions returns all active RTP sessions
func (m *Manager) GetSessions() map[string]*Session {
	m.sessionMutex.RLock()
	defer m.sessionMutex.RUnlock()

	sessions := make(map[string]*Session)
	for k, v := range m.sessions {
		sessions[k] = v
	}
	return sessions
}

// handlePackets handles incoming RTP packets for a session
func (m *Manager) handlePackets(session *Session) {
	log := m.logger.NewLogger("rtp-handler")
	buffer := make([]byte, 1500) // Standard MTU size

	for {
		n, remoteAddr, err := session.LocalConn.ReadFromUDP(buffer)
		if err != nil {
			// Connection closed or error
			log.Debugf("RTP read error for session %s: %v", session.ID, err)
			break
		}

		// Parse RTP packet
		packet := &rtp.Packet{}
		if err := packet.Unmarshal(buffer[:n]); err != nil {
			log.Warnf("Failed to parse RTP packet for session %s: %v", session.ID, err)
			continue
		}

		// Update session statistics
		session.mutex.Lock()
		session.PacketsRecv++
		session.BytesRecv += uint64(n)
		session.mutex.Unlock()

		// Process the packet
		m.processRTPPacket(session, packet, remoteAddr)
	}
}

// processRTPPacket processes an incoming RTP packet
func (m *Manager) processRTPPacket(session *Session, packet *rtp.Packet, remoteAddr *net.UDPAddr) {
	log := m.logger.NewLogger("rtp-processor")

	// Log packet details (in debug mode)
	log.Debugf("RTP packet: Session=%s, SSRC=%d, PayloadType=%d, SeqNum=%d, Timestamp=%d, Size=%d",
		session.ID, packet.SSRC, packet.PayloadType, packet.SequenceNumber, packet.Timestamp, len(packet.Payload))

	// TODO: Implement codec-specific processing
	// This would handle different codec types (PCMU, PCMA, Opus, L16)
	switch packet.PayloadType {
	case 0: // PCMU
		m.processPCMU(session, packet)
	case 8: // PCMA
		m.processPCMA(session, packet)
	case 96: // Opus (dynamic)
		m.processOpus(session, packet)
	case 97: // L16 (dynamic)
		m.processL16(session, packet)
	default:
		log.Warnf("Unsupported payload type: %d", packet.PayloadType)
	}
}

// processPCMU processes PCMU codec packets
func (m *Manager) processPCMU(session *Session, packet *rtp.Packet) {
	// TODO: Implement PCMU processing
	// This would decode PCMU audio data
}

// processPCMA processes PCMA codec packets
func (m *Manager) processPCMA(session *Session, packet *rtp.Packet) {
	// TODO: Implement PCMA processing
	// This would decode PCMA audio data
}

// processOpus processes Opus codec packets
func (m *Manager) processOpus(session *Session, packet *rtp.Packet) {
	// TODO: Implement Opus processing
	// This would decode Opus audio data
}

// processL16 processes L16 codec packets
func (m *Manager) processL16(session *Session, packet *rtp.Packet) {
	// TODO: Implement L16 processing
	// This would process linear PCM audio data
}

// SendRTPPacket sends an RTP packet to the remote endpoint
func (m *Manager) SendRTPPacket(sessionID string, packet *rtp.Packet) error {
	session, exists := m.GetSession(sessionID)
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Marshal packet
	data, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal RTP packet: %w", err)
	}

	// Convert remote address to UDP address
	udpAddr, ok := session.RemoteAddr.(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("invalid remote address type")
	}

	// Send packet
	_, err = session.LocalConn.WriteToUDP(data, udpAddr)
	if err != nil {
		return fmt.Errorf("failed to send RTP packet: %w", err)
	}

	// Update statistics
	session.mutex.Lock()
	session.PacketsSent++
	session.BytesSent += uint64(len(data))
	session.mutex.Unlock()

	return nil
}

// LocalPort returns the local port for a session
func (s *Session) LocalPort() int {
	return s.LocalAddr.Port
}

// GetStats returns session statistics
func (s *Session) GetStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return map[string]interface{}{
		"session_id":    s.ID,
		"local_addr":    s.LocalAddr.String(),
		"remote_addr":   s.RemoteAddr.String(),
		"start_time":    s.StartTime,
		"duration":      time.Since(s.StartTime),
		"packets_sent":  s.PacketsSent,
		"bytes_sent":    s.BytesSent,
		"packets_recv":  s.PacketsRecv,
		"bytes_recv":    s.BytesRecv,
	}
}

// GetStats returns manager statistics
func (m *Manager) GetStats() map[string]interface{} {
	m.sessionMutex.RLock()
	defer m.sessionMutex.RUnlock()

	return map[string]interface{}{
		"active_sessions": len(m.sessions),
		"used_ports":      m.portPool.GetUsedPortsCount(),
		"port_range":      fmt.Sprintf("%d-%d", m.portPool.minPort, m.portPool.maxPort),
	}
}