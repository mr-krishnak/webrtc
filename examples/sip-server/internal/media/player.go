// Package media provides media processing capabilities
package media

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/examples/sip-server/internal/config"
	rtpManager "github.com/pion/webrtc/v4/examples/sip-server/internal/rtp"
)

// Player handles media playback to RTP sessions
type Player struct {
	config    *config.Config
	logger    logging.LoggerFactory
	rtpMgr    *rtpManager.Manager
	sessions  map[string]*PlaybackSession
	mutex     sync.RWMutex
}

// PlaybackSession represents an active media playback session
type PlaybackSession struct {
	ID           string
	SessionID    string
	FilePath     string
	StartTime    time.Time
	Position     int64
	IsPlaying    bool
	IsPaused     bool
	Loop         bool
	file         *os.File
	mutex        sync.RWMutex
	stopChan     chan struct{}
}

// MediaFile represents a media file with metadata
type MediaFile struct {
	Path     string
	Name     string
	Size     int64
	Duration time.Duration
	Format   string
	Codec    string
}

// NewPlayer creates a new media player
func NewPlayer(cfg *config.Config, logger logging.LoggerFactory, rtpMgr *rtpManager.Manager) *Player {
	return &Player{
		config:   cfg,
		logger:   logger,
		rtpMgr:   rtpMgr,
		sessions: make(map[string]*PlaybackSession),
	}
}

// PlayFile plays a media file to an RTP session
func (p *Player) PlayFile(sessionID, filePath string, loop bool) (*PlaybackSession, error) {
	log := p.logger.NewLogger("media-player")

	// Check if RTP session exists
	if _, exists := p.rtpMgr.GetSession(sessionID); !exists {
		return nil, fmt.Errorf("RTP session not found: %s", sessionID)
	}

	// Resolve full file path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(p.config.Media.MediaFilesPath, filePath)
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("media file not found: %s", filePath)
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open media file: %w", err)
	}

	playbackID := fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
	session := &PlaybackSession{
		ID:        playbackID,
		SessionID: sessionID,
		FilePath:  filePath,
		StartTime: time.Now(),
		Position:  0,
		IsPlaying: true,
		IsPaused:  false,
		Loop:      loop,
		file:      file,
		stopChan:  make(chan struct{}),
	}

	// Store session
	p.mutex.Lock()
	p.sessions[playbackID] = session
	p.mutex.Unlock()

	log.Infof("Starting playback: %s -> %s (loop=%t)", filePath, sessionID, loop)

	// Start playback in goroutine
	go p.playback(session)

	return session, nil
}

// PlayGreeting plays the configured greeting file
func (p *Player) PlayGreeting(sessionID string) (*PlaybackSession, error) {
	return p.PlayFile(sessionID, p.config.Media.GreetingFile, false)
}

// StopPlayback stops a playback session
func (p *Player) StopPlayback(playbackID string) error {
	p.mutex.RLock()
	session, exists := p.sessions[playbackID]
	p.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("playback session not found: %s", playbackID)
	}

	session.mutex.Lock()
	if session.IsPlaying {
		close(session.stopChan)
		session.IsPlaying = false
	}
	session.mutex.Unlock()

	return nil
}

// PausePlayback pauses a playback session
func (p *Player) PausePlayback(playbackID string) error {
	p.mutex.RLock()
	session, exists := p.sessions[playbackID]
	p.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("playback session not found: %s", playbackID)
	}

	session.mutex.Lock()
	session.IsPaused = true
	session.mutex.Unlock()

	return nil
}

// ResumePlayback resumes a paused playback session
func (p *Player) ResumePlayback(playbackID string) error {
	p.mutex.RLock()
	session, exists := p.sessions[playbackID]
	p.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("playback session not found: %s", playbackID)
	}

	session.mutex.Lock()
	session.IsPaused = false
	session.mutex.Unlock()

	return nil
}

// playback handles the actual media playback
func (p *Player) playback(session *PlaybackSession) {
	log := p.logger.NewLogger("media-playback")
	defer func() {
		session.file.Close()
		p.mutex.Lock()
		delete(p.sessions, session.ID)
		p.mutex.Unlock()
		log.Infof("Playback finished: %s", session.ID)
	}()

	// Simple implementation: read file in chunks and send as RTP
	// In a real implementation, this would parse the audio format
	// and properly encode it for RTP transmission
	
	buffer := make([]byte, 160) // 20ms of 8kHz PCMU audio
	sequenceNumber := uint16(0)
	timestamp := uint32(0)
	ssrc := uint32(12345) // Should be unique per session

	ticker := time.NewTicker(20 * time.Millisecond) // 20ms intervals
	defer ticker.Stop()

	for {
		select {
		case <-session.stopChan:
			return
		case <-ticker.C:
			session.mutex.RLock()
			isPaused := session.IsPaused
			session.mutex.RUnlock()

			if isPaused {
				continue
			}

			// Read audio data
			n, err := session.file.Read(buffer)
			if err != nil {
				if err == io.EOF {
					if session.Loop {
						// Reset file position for looping
						session.file.Seek(0, 0)
						session.mutex.Lock()
						session.Position = 0
						session.mutex.Unlock()
						continue
					} else {
						// End of file, stop playback
						return
					}
				}
				log.Errorf("Error reading media file: %v", err)
				return
			}

			// Update position
			session.mutex.Lock()
			session.Position += int64(n)
			session.mutex.Unlock()

			// Create RTP packet
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Padding:        false,
					Extension:      false,
					Marker:         false,
					PayloadType:    0, // PCMU
					SequenceNumber: sequenceNumber,
					Timestamp:      timestamp,
					SSRC:           ssrc,
				},
				Payload: buffer[:n],
			}

			// Send RTP packet
			if err := p.rtpMgr.SendRTPPacket(session.SessionID, packet); err != nil {
				log.Errorf("Failed to send RTP packet: %v", err)
				return
			}

			sequenceNumber++
			timestamp += 160 // 20ms at 8kHz
		}
	}
}

// GetPlaybackSessions returns all active playback sessions
func (p *Player) GetPlaybackSessions() map[string]*PlaybackSession {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	sessions := make(map[string]*PlaybackSession)
	for k, v := range p.sessions {
		sessions[k] = v
	}
	return sessions
}

// GetPlaybackSession returns a specific playback session
func (p *Player) GetPlaybackSession(playbackID string) (*PlaybackSession, bool) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	session, exists := p.sessions[playbackID]
	return session, exists
}

// ListMediaFiles lists available media files
func (p *Player) ListMediaFiles() ([]MediaFile, error) {
	var files []MediaFile

	err := filepath.Walk(p.config.Media.MediaFilesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Check if it's an audio file (simple check by extension)
			ext := filepath.Ext(path)
			if isAudioFile(ext) {
				relPath, _ := filepath.Rel(p.config.Media.MediaFilesPath, path)
				file := MediaFile{
					Path:   relPath,
					Name:   info.Name(),
					Size:   info.Size(),
					Format: ext,
				}
				files = append(files, file)
			}
		}

		return nil
	})

	return files, err
}

// isAudioFile checks if a file extension indicates an audio file
func isAudioFile(ext string) bool {
	audioExts := []string{".wav", ".mp3", ".flac", ".ogg", ".aac", ".m4a", ".au", ".raw"}
	for _, audioExt := range audioExts {
		if ext == audioExt {
			return true
		}
	}
	return false
}

// GetStats returns playback statistics
func (p *Player) GetStats() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	stats := map[string]interface{}{
		"active_sessions": len(p.sessions),
		"sessions":        make([]map[string]interface{}, 0, len(p.sessions)),
	}

	for _, session := range p.sessions {
		session.mutex.RLock()
		sessionStats := map[string]interface{}{
			"id":         session.ID,
			"session_id": session.SessionID,
			"file_path":  session.FilePath,
			"start_time": session.StartTime,
			"position":   session.Position,
			"is_playing": session.IsPlaying,
			"is_paused":  session.IsPaused,
			"loop":       session.Loop,
			"duration":   time.Since(session.StartTime),
		}
		session.mutex.RUnlock()
		stats["sessions"] = append(stats["sessions"].([]map[string]interface{}), sessionStats)
	}

	return stats
}

// StopAllPlayback stops all active playback sessions
func (p *Player) StopAllPlayback() {
	p.mutex.RLock()
	sessions := make([]*PlaybackSession, 0, len(p.sessions))
	for _, session := range p.sessions {
		sessions = append(sessions, session)
	}
	p.mutex.RUnlock()

	for _, session := range sessions {
		p.StopPlayback(session.ID)
	}
}