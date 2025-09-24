// Package codec provides codec support for the SIP server
package codec

import (
	"fmt"

	"github.com/pion/webrtc/v4"
)

// SupportedCodec represents a supported codec
type SupportedCodec struct {
	Name        string
	PayloadType uint8
	ClockRate   uint32
	Channels    uint16
	FMTP        string
	Capability  webrtc.RTPCodecCapability
}

// CodecManager manages supported codecs
type CodecManager struct {
	codecs map[string]*SupportedCodec
}

// NewCodecManager creates a new codec manager
func NewCodecManager() *CodecManager {
	cm := &CodecManager{
		codecs: make(map[string]*SupportedCodec),
	}
	
	// Initialize default codecs
	cm.initializeDefaultCodecs()
	
	return cm
}

// initializeDefaultCodecs initializes the default supported codecs
func (cm *CodecManager) initializeDefaultCodecs() {
	// PCMU (G.711 μ-law)
	cm.codecs["PCMU"] = &SupportedCodec{
		Name:        "PCMU",
		PayloadType: 0,
		ClockRate:   8000,
		Channels:    1,
		FMTP:        "",
		Capability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  1,
		},
	}

	// PCMA (G.711 A-law)
	cm.codecs["PCMA"] = &SupportedCodec{
		Name:        "PCMA",
		PayloadType: 8,
		ClockRate:   8000,
		Channels:    1,
		FMTP:        "",
		Capability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
			Channels:  1,
		},
	}

	// Opus
	cm.codecs["Opus"] = &SupportedCodec{
		Name:        "Opus",
		PayloadType: 96, // Dynamic payload type
		ClockRate:   48000,
		Channels:    2,
		FMTP:        "sprop-stereo=1",
		Capability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     2,
			SDPFmtpLine:  "sprop-stereo=1",
			RTCPFeedback: nil,
		},
	}

	// L16 (Linear PCM)
	cm.codecs["L16"] = &SupportedCodec{
		Name:        "L16",
		PayloadType: 97, // Dynamic payload type
		ClockRate:   44100,
		Channels:    2,
		FMTP:        "",
		Capability: webrtc.RTPCodecCapability{
			MimeType:  "audio/L16",
			ClockRate: 44100,
			Channels:  2,
		},
	}
}

// GetCodec returns a codec by name
func (cm *CodecManager) GetCodec(name string) (*SupportedCodec, error) {
	codec, exists := cm.codecs[name]
	if !exists {
		return nil, fmt.Errorf("codec not supported: %s", name)
	}
	return codec, nil
}

// GetSupportedCodecs returns all supported codecs
func (cm *CodecManager) GetSupportedCodecs() map[string]*SupportedCodec {
	codecs := make(map[string]*SupportedCodec)
	for k, v := range cm.codecs {
		codecs[k] = v
	}
	return codecs
}

// IsCodecSupported checks if a codec is supported
func (cm *CodecManager) IsCodecSupported(name string) bool {
	_, exists := cm.codecs[name]
	return exists
}

// GetCodecByPayloadType returns a codec by payload type
func (cm *CodecManager) GetCodecByPayloadType(pt uint8) (*SupportedCodec, error) {
	for _, codec := range cm.codecs {
		if codec.PayloadType == pt {
			return codec, nil
		}
	}
	return nil, fmt.Errorf("codec not found for payload type: %d", pt)
}

// RegisterCodec registers a new codec
func (cm *CodecManager) RegisterCodec(codec *SupportedCodec) error {
	if codec.Name == "" {
		return fmt.Errorf("codec name cannot be empty")
	}
	
	// Check if payload type is already used
	for name, existing := range cm.codecs {
		if existing.PayloadType == codec.PayloadType && name != codec.Name {
			return fmt.Errorf("payload type %d already used by codec %s", codec.PayloadType, name)
		}
	}
	
	cm.codecs[codec.Name] = codec
	return nil
}

// UnregisterCodec unregisters a codec
func (cm *CodecManager) UnregisterCodec(name string) error {
	if _, exists := cm.codecs[name]; !exists {
		return fmt.Errorf("codec not found: %s", name)
	}
	
	delete(cm.codecs, name)
	return nil
}

// NegotiateCodecs negotiates codecs based on offered codecs
func (cm *CodecManager) NegotiateCodecs(offeredCodecs []string) ([]*SupportedCodec, error) {
	var negotiated []*SupportedCodec
	
	for _, offered := range offeredCodecs {
		if codec, exists := cm.codecs[offered]; exists {
			negotiated = append(negotiated, codec)
		}
	}
	
	if len(negotiated) == 0 {
		return nil, fmt.Errorf("no common codecs found")
	}
	
	return negotiated, nil
}

// GenerateRTPCodecParameters generates RTP codec parameters for WebRTC
func (cm *CodecManager) GenerateRTPCodecParameters() []webrtc.RTPCodecParameters {
	var params []webrtc.RTPCodecParameters
	
	for _, codec := range cm.codecs {
		param := webrtc.RTPCodecParameters{
			RTPCodecCapability: codec.Capability,
			PayloadType:        webrtc.PayloadType(codec.PayloadType),
		}
		params = append(params, param)
	}
	
	return params
}

// GetCodecInfo returns formatted codec information
func (cm *CodecManager) GetCodecInfo() map[string]interface{} {
	info := make(map[string]interface{})
	
	for name, codec := range cm.codecs {
		info[name] = map[string]interface{}{
			"payload_type": codec.PayloadType,
			"clock_rate":   codec.ClockRate,
			"channels":     codec.Channels,
			"fmtp":         codec.FMTP,
			"mime_type":    codec.Capability.MimeType,
		}
	}
	
	return info
}

// Decoder interface for codec decoders
type Decoder interface {
	Decode(data []byte) ([]byte, error)
	Reset() error
}

// Encoder interface for codec encoders
type Encoder interface {
	Encode(data []byte) ([]byte, error)
	Reset() error
}

// PCMUDecoder implements PCMU decoding
type PCMUDecoder struct{}

// NewPCMUDecoder creates a new PCMU decoder
func NewPCMUDecoder() *PCMUDecoder {
	return &PCMUDecoder{}
}

// Decode decodes PCMU data to linear PCM
func (d *PCMUDecoder) Decode(data []byte) ([]byte, error) {
	// μ-law to linear PCM conversion
	pcm := make([]byte, len(data)*2) // 16-bit output
	
	for i, sample := range data {
		// μ-law decompression algorithm
		sample = ^sample
		sign := sample & 0x80
		exponent := (sample >> 4) & 0x07
		mantissa := sample & 0x0F
		
		linear := int16(mantissa<<4 | 0x08)
		linear <<= exponent
		linear -= 0x84
		
		if sign != 0 {
			linear = -linear
		}
		
		// Convert to bytes (little endian)
		pcm[i*2] = byte(linear & 0xFF)
		pcm[i*2+1] = byte(linear >> 8)
	}
	
	return pcm, nil
}

// Reset resets the decoder state
func (d *PCMUDecoder) Reset() error {
	return nil
}

// PCMADecoder implements PCMA decoding
type PCMADecoder struct{}

// NewPCMADecoder creates a new PCMA decoder
func NewPCMADecoder() *PCMADecoder {
	return &PCMADecoder{}
}

// Decode decodes PCMA data to linear PCM
func (d *PCMADecoder) Decode(data []byte) ([]byte, error) {
	// A-law to linear PCM conversion
	pcm := make([]byte, len(data)*2) // 16-bit output
	
	for i, sample := range data {
		// A-law decompression algorithm
		sample ^= 0x55
		sign := sample & 0x80
		exponent := (sample >> 4) & 0x07
		mantissa := sample & 0x0F
		
		var linear int16
		if exponent == 0 {
			linear = int16(mantissa << 4)
		} else {
			linear = int16((mantissa|0x10) << (exponent + 3))
		}
		
		if sign != 0 {
			linear = -linear
		}
		
		// Convert to bytes (little endian)
		pcm[i*2] = byte(linear & 0xFF)
		pcm[i*2+1] = byte(linear >> 8)
	}
	
	return pcm, nil
}

// Reset resets the decoder state
func (d *PCMADecoder) Reset() error {
	return nil
}

// CreateDecoder creates a decoder for the specified codec
func (cm *CodecManager) CreateDecoder(codecName string) (Decoder, error) {
	switch codecName {
	case "PCMU":
		return NewPCMUDecoder(), nil
	case "PCMA":
		return NewPCMADecoder(), nil
	case "Opus":
		// TODO: Implement Opus decoder
		return nil, fmt.Errorf("Opus decoder not implemented")
	case "L16":
		// L16 doesn't need decoding (already linear PCM)
		return nil, fmt.Errorf("L16 doesn't require decoding")
	default:
		return nil, fmt.Errorf("decoder not available for codec: %s", codecName)
	}
}