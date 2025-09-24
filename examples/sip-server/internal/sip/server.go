// Package sip provides SIP server functionality
package sip

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v4/examples/sip-server/internal/config"
	"github.com/pion/webrtc/v4/examples/sip-server/internal/rtp"
)

// Server represents the SIP server
type Server struct {
	config       *config.Config
	logger       logging.LoggerFactory
	conn         *net.UDPConn
	rtpManager   *rtp.Manager
	activeCalls  map[string]*Call
	callsMutex   sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// Call represents an active SIP call
type Call struct {
	ID         string
	CallID     string
	FromURI    string
	ToURI      string
	DID        string
	RemoteAddr net.Addr
	StartTime  time.Time
	Status     CallStatus
	RTPSession *rtp.Session
	mutex      sync.RWMutex
}

// CallStatus represents the status of a call
type CallStatus int

const (
	CallStatusIncoming CallStatus = iota
	CallStatusConnected
	CallStatusTerminated
)

// String returns string representation of CallStatus
func (cs CallStatus) String() string {
	switch cs {
	case CallStatusIncoming:
		return "incoming"
	case CallStatusConnected:
		return "connected"
	case CallStatusTerminated:
		return "terminated"
	default:
		return "unknown"
	}
}

// SIPMessage represents a basic SIP message
type SIPMessage struct {
	Method    string
	RequestURI string
	Version   string
	Headers   map[string]string
	Body      string
	Raw       string
}

// NewServer creates a new SIP server instance
func NewServer(cfg *config.Config, logger logging.LoggerFactory, rtpMgr *rtp.Manager) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	server := &Server{
		config:      cfg,
		logger:      logger,
		rtpManager:  rtpMgr,
		activeCalls: make(map[string]*Call),
		ctx:         ctx,
		cancel:      cancel,
	}

	return server, nil
}

// Start starts the SIP server
func (s *Server) Start() error {
	log := s.logger.NewLogger("sip-server")
	log.Infof("Starting SIP server on %s:%d", s.config.Server.BindAddress, s.config.Server.SIPPort)

	// Create UDP address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", s.config.Server.BindAddress, s.config.Server.SIPPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// Listen on UDP
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}
	s.conn = conn

	// Start handling messages
	go s.handleMessages()

	// Start call cleanup routine
	go s.callCleanupRoutine()

	return nil
}

// Stop stops the SIP server
func (s *Server) Stop() error {
	log := s.logger.NewLogger("sip-server")
	log.Info("Stopping SIP server")

	s.cancel()

	// Terminate all active calls
	s.callsMutex.Lock()
	for _, call := range s.activeCalls {
		s.terminateCall(call, "Server shutdown")
	}
	s.callsMutex.Unlock()

	// Close UDP connection
	if s.conn != nil {
		return s.conn.Close()
	}

	return nil
}

// handleMessages handles incoming SIP messages
func (s *Server) handleMessages() {
	log := s.logger.NewLogger("sip-handler")
	buffer := make([]byte, 4096)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			n, remoteAddr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				if s.ctx.Err() != nil {
					return // Server is shutting down
				}
				log.Errorf("Error reading UDP message: %v", err)
				continue
			}

			// Parse SIP message
			msg, err := s.parseSIPMessage(string(buffer[:n]))
			if err != nil {
				log.Warnf("Failed to parse SIP message: %v", err)
				continue
			}

			// Handle the message
			s.handleSIPMessage(msg, remoteAddr)
		}
	}
}

// parseSIPMessage parses a basic SIP message
func (s *Server) parseSIPMessage(data string) (*SIPMessage, error) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty message")
	}

	// Parse request line
	requestLine := strings.TrimSpace(lines[0])
	parts := strings.Split(requestLine, " ")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid request line: %s", requestLine)
	}

	msg := &SIPMessage{
		Method:     parts[0],
		RequestURI: parts[1],
		Version:    parts[2],
		Headers:    make(map[string]string),
		Raw:        data,
	}

	// Parse headers
	bodyStart := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			bodyStart = i + 1
			break
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		msg.Headers[strings.ToLower(key)] = value
	}

	// Parse body
	if bodyStart != -1 && bodyStart < len(lines) {
		msg.Body = strings.Join(lines[bodyStart:], "\n")
	}

	return msg, nil
}

// handleSIPMessage handles a parsed SIP message
func (s *Server) handleSIPMessage(msg *SIPMessage, remoteAddr *net.UDPAddr) {
	log := s.logger.NewLogger("sip-message")

	switch msg.Method {
	case "INVITE":
		s.handleInvite(msg, remoteAddr)
	case "ACK":
		s.handleAck(msg, remoteAddr)
	case "BYE":
		s.handleBye(msg, remoteAddr)
	case "CANCEL":
		s.handleCancel(msg, remoteAddr)
	default:
		log.Warnf("Unsupported SIP method: %s", msg.Method)
		s.sendResponse(msg, remoteAddr, 405, "Method Not Allowed")
	}
}

// handleInvite handles INVITE requests
func (s *Server) handleInvite(msg *SIPMessage, remoteAddr *net.UDPAddr) {
	log := s.logger.NewLogger("sip-invite")

	// Check IP whitelist
	if !s.config.IsIPAllowed(remoteAddr.IP) {
		log.Warnf("Rejecting call from non-whitelisted IP: %s", remoteAddr.IP)
		s.sendResponse(msg, remoteAddr, 403, "Forbidden")
		return
	}

	// Extract DID from To header
	toHeader := msg.Headers["to"]
	did := s.extractDIDFromHeader(toHeader)
	if !s.config.IsDIDValid(did) {
		log.Warnf("Invalid DID requested: %s", did)
		s.sendResponse(msg, remoteAddr, 404, "Not Found")
		return
	}

	// Check concurrent call limit
	s.callsMutex.RLock()
	callCount := len(s.activeCalls)
	s.callsMutex.RUnlock()

	if callCount >= s.config.Concurrency.MaxConcurrentCalls {
		log.Warnf("Maximum concurrent calls reached: %d", callCount)
		s.sendResponse(msg, remoteAddr, 503, "Service Unavailable")
		return
	}

	// Create new call
	call := &Call{
		ID:         uuid.New().String(),
		CallID:     msg.Headers["call-id"],
		FromURI:    msg.Headers["from"],
		ToURI:      msg.Headers["to"],
		DID:        did,
		RemoteAddr: remoteAddr,
		StartTime:  time.Now(),
		Status:     CallStatusIncoming,
	}

	log.Infof("New incoming call: ID=%s, From=%s, To=%s, DID=%s, RemoteIP=%s",
		call.ID, call.FromURI, call.ToURI, call.DID, remoteAddr.IP)

	// Create RTP session
	rtpSession, err := s.rtpManager.CreateSession(call.ID, remoteAddr)
	if err != nil {
		log.Errorf("Failed to create RTP session: %v", err)
		s.sendResponse(msg, remoteAddr, 500, "Internal Server Error")
		return
	}
	call.RTPSession = rtpSession

	// Store call
	s.callsMutex.Lock()
	s.activeCalls[call.ID] = call
	s.callsMutex.Unlock()

	// Send 100 Trying
	s.sendResponse(msg, remoteAddr, 100, "Trying")

	// Process SDP and send 200 OK
	s.processSDP(call, msg, remoteAddr)
}

// handleAck handles ACK requests
func (s *Server) handleAck(msg *SIPMessage, remoteAddr *net.UDPAddr) {
	log := s.logger.NewLogger("sip-ack")

	callID := msg.Headers["call-id"]
	call := s.findCallByCallID(callID)
	if call == nil {
		log.Warnf("ACK for unknown call: %s", callID)
		return
	}

	call.mutex.Lock()
	call.Status = CallStatusConnected
	call.mutex.Unlock()

	log.Infof("ACK received for call: %s - call connected", call.ID)

	// Start media processing
	go s.startMediaProcessing(call)
}

// handleBye handles BYE requests
func (s *Server) handleBye(msg *SIPMessage, remoteAddr *net.UDPAddr) {
	log := s.logger.NewLogger("sip-bye")

	callID := msg.Headers["call-id"]
	call := s.findCallByCallID(callID)
	if call == nil {
		log.Warnf("BYE for unknown call: %s", callID)
		s.sendResponse(msg, remoteAddr, 404, "Call Not Found")
		return
	}

	log.Infof("BYE received for call: %s", call.ID)

	// Send 200 OK
	s.sendResponse(msg, remoteAddr, 200, "OK")

	// Terminate the call
	s.terminateCall(call, "BYE received")
}

// handleCancel handles CANCEL requests
func (s *Server) handleCancel(msg *SIPMessage, remoteAddr *net.UDPAddr) {
	log := s.logger.NewLogger("sip-cancel")

	callID := msg.Headers["call-id"]
	call := s.findCallByCallID(callID)
	if call == nil {
		log.Warnf("CANCEL for unknown call: %s", callID)
		s.sendResponse(msg, remoteAddr, 404, "Call Not Found")
		return
	}

	log.Infof("CANCEL received for call: %s", call.ID)

	// Send 200 OK for CANCEL
	s.sendResponse(msg, remoteAddr, 200, "OK")

	// Terminate the call
	s.terminateCall(call, "CANCEL received")
}

// sendResponse sends a SIP response
func (s *Server) sendResponse(msg *SIPMessage, remoteAddr *net.UDPAddr, statusCode int, reasonPhrase string) {
	log := s.logger.NewLogger("sip-response")

	// Create response
	response := fmt.Sprintf("SIP/2.0 %d %s\r\n", statusCode, reasonPhrase)
	response += fmt.Sprintf("Via: %s\r\n", msg.Headers["via"])
	response += fmt.Sprintf("From: %s\r\n", msg.Headers["from"])
	response += fmt.Sprintf("To: %s\r\n", msg.Headers["to"])
	response += fmt.Sprintf("Call-ID: %s\r\n", msg.Headers["call-id"])
	response += fmt.Sprintf("CSeq: %s\r\n", msg.Headers["cseq"])

	if statusCode == 200 && msg.Method == "INVITE" {
		// Add SDP for 200 OK to INVITE
		callID := msg.Headers["call-id"]
		call := s.findCallByCallID(callID)
		if call != nil {
			sdp := s.generateSDPAnswer(call)
			response += "Content-Type: application/sdp\r\n"
			response += fmt.Sprintf("Content-Length: %d\r\n", len(sdp))
			response += "\r\n"
			response += sdp
		}
	} else {
		response += "Content-Length: 0\r\n"
		response += "\r\n"
	}

	// Send response
	_, err := s.conn.WriteToUDP([]byte(response), remoteAddr)
	if err != nil {
		log.Errorf("Failed to send response: %v", err)
	}
}

// terminateCall terminates a call and cleans up resources
func (s *Server) terminateCall(call *Call, reason string) {
	log := s.logger.NewLogger("sip-terminate")
	
	call.mutex.Lock()
	if call.Status == CallStatusTerminated {
		call.mutex.Unlock()
		return
	}
	call.Status = CallStatusTerminated
	call.mutex.Unlock()

	log.Infof("Terminating call: %s, reason: %s", call.ID, reason)

	// Close RTP session
	if call.RTPSession != nil {
		s.rtpManager.CloseSession(call.ID)
	}

	// Remove from active calls
	s.callsMutex.Lock()
	delete(s.activeCalls, call.ID)
	s.callsMutex.Unlock()
}

// callCleanupRoutine periodically cleans up expired calls
func (s *Server) callCleanupRoutine() {
	log := s.logger.NewLogger("sip-cleanup")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	timeout := time.Duration(s.config.Concurrency.CallTimeoutSeconds) * time.Second

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			var expiredCalls []*Call

			s.callsMutex.RLock()
			for _, call := range s.activeCalls {
				if now.Sub(call.StartTime) > timeout {
					expiredCalls = append(expiredCalls, call)
				}
			}
			s.callsMutex.RUnlock()

			for _, call := range expiredCalls {
				log.Infof("Call timeout: %s", call.ID)
				s.terminateCall(call, "Timeout")
			}
		}
	}
}

// processSDP processes SDP offer and generates answer
func (s *Server) processSDP(call *Call, msg *SIPMessage, remoteAddr *net.UDPAddr) {
	// TODO: Implement SDP processing
	// This would parse the SDP offer, negotiate codecs, and generate an SDP answer

	// For now, send a simple 200 OK with minimal SDP
	s.sendResponse(msg, remoteAddr, 200, "OK")
}

// generateSDPAnswer generates a simple SDP answer
func (s *Server) generateSDPAnswer(call *Call) string {
	// This is a simplified SDP answer
	// In a real implementation, this would be more sophisticated
	rtpPort := call.RTPSession.LocalPort()

	return fmt.Sprintf(`v=0
o=sipserver 123456 654321 IN IP4 %s
s=SIP Server Session
c=IN IP4 %s
t=0 0
m=audio %d RTP/AVP 0 8 96
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:96 opus/48000/2
a=sendrecv
`, s.config.Server.BindAddress, s.config.Server.BindAddress, rtpPort)
}

// startMediaProcessing starts media processing for a connected call
func (s *Server) startMediaProcessing(call *Call) {
	log := s.logger.NewLogger("sip-media")
	log.Infof("Starting media processing for call: %s", call.ID)

	// TODO: Implement media processing
	// This would handle RTP packets, play greeting files, etc.
}

// findCallByCallID finds a call by Call-ID
func (s *Server) findCallByCallID(callID string) *Call {
	s.callsMutex.RLock()
	defer s.callsMutex.RUnlock()

	for _, call := range s.activeCalls {
		if call.CallID == callID {
			return call
		}
	}
	return nil
}

// extractDIDFromHeader extracts DID from SIP header
func (s *Server) extractDIDFromHeader(header string) string {
	// Simple extraction - look for sip:user@domain pattern
	if strings.Contains(header, "sip:") {
		start := strings.Index(header, "sip:") + 4
		end := strings.Index(header[start:], "@")
		if end != -1 {
			return header[start : start+end]
		}
	}
	return ""
}

// GetActiveCalls returns a copy of active calls map
func (s *Server) GetActiveCalls() map[string]*Call {
	s.callsMutex.RLock()
	defer s.callsMutex.RUnlock()
	
	calls := make(map[string]*Call)
	for k, v := range s.activeCalls {
		calls[k] = v
	}
	return calls
}

// GetCallStats returns statistics about calls
func (s *Server) GetCallStats() map[string]interface{} {
	s.callsMutex.RLock()
	defer s.callsMutex.RUnlock()
	
	stats := map[string]interface{}{
		"active_calls": len(s.activeCalls),
		"max_calls":    s.config.Concurrency.MaxConcurrentCalls,
	}
	
	statusCounts := make(map[string]int)
	for _, call := range s.activeCalls {
		statusCounts[call.Status.String()]++
	}
	stats["call_status_counts"] = statusCounts
	
	return stats
}