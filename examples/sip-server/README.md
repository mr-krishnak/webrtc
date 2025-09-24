# SIP Server Application

A comprehensive Go SIP server application built with Pion WebRTC and Diago SIP library, featuring advanced media processing and transcoding capabilities.

## Features

### Core Components
- **SIP Server**: Listens on port 5060 using diago for SIP/dialog control
- **RTP Handling**: Uses pion/rtp for RTP packet handling with dynamic port allocation (10000-40000)
- **Codec Support**: Implements PCMU, PCMA, Opus, and L16 codecs using pion/webrtc codec packages

### Security & Validation
- **IP Whitelisting**: Enforces caller IP white-listing, rejects calls from non-whitelisted IPs
- **DID Validation**: Checks if target DID exists, sends hangup/reject for invalid DIDs

### Media Processing
- **Media Negotiation**: Accepts calls and negotiates media with supported codecs
- **FFmpeg Integration**: Uses ffmpeg for transcoding when necessary
- **Media Playback**: Plays media files (greeting/prompt) to callers

### Concurrency
- Handles multiple concurrent calls on same DID and different DIDs
- Proper goroutine management and resource cleanup

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   SIP Server    │────│   RTP Manager   │────│  Media Player   │
│   (Port 5060)   │    │  (Port 10000+)  │    │  (Playback)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ Codec Manager   │    │   Config Mgmt   │    │FFmpeg Transcoder│
│ (PCMU/PCMA/Opus)│    │   (YAML/JSON)   │    │   (Optional)    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Installation

### Prerequisites
- Go 1.21 or later
- FFmpeg (optional, for transcoding)

### Build from Source

```bash
cd examples/sip-server
go mod tidy
go build -o bin/sip-server ./cmd/sip-server
```

## Configuration

The server uses a YAML configuration file. See `configs/config.yaml` for an example:

```yaml
server:
  sip_port: 5060
  rtp_port_range:
    min: 10000
    max: 40000
  bind_address: "0.0.0.0"

security:
  ip_whitelist:
    - "127.0.0.1"
    - "192.168.1.0/24"
    - "10.0.0.0/8"

dids:
  valid_dids:
    - "1001"
    - "1002" 
    - "1003"
    - "support"
    - "sales"

media:
  supported_codecs:
    - "PCMU"
    - "PCMA" 
    - "Opus"
    - "L16"
  media_files_path: "./media-files"
  greeting_file: "greeting.wav"
  
ffmpeg:
  path: "/usr/bin/ffmpeg"
  enabled: true

logging:
  level: "info"
  format: "json"
  file: "sip-server.log"

concurrency:
  max_concurrent_calls: 100
  call_timeout_seconds: 300
```

## Usage

### Basic Usage

```bash
# Start with default configuration
./bin/sip-server

# Start with custom configuration
./bin/sip-server -config /path/to/config.yaml

# Set log level
./bin/sip-server -log-level debug

# Show version
./bin/sip-server -version
```

### Testing the Server

You can test the SIP server using any SIP client such as:
- SIP softphones (X-Lite, Linphone, etc.)
- SIP testing tools (SIPp, Asterisk, etc.)

Example SIP call to DID "1001":
```
sip:1001@your-server-ip:5060
```

## API Documentation

### SIP Methods Supported

- **INVITE**: Initiates a call, validates IP and DID, negotiates media
- **ACK**: Confirms call establishment
- **BYE**: Terminates an active call
- **CANCEL**: Cancels a pending call

### Call Flow

1. **INVITE** received → IP whitelist check → DID validation → Media negotiation
2. **100 Trying** sent → RTP session created → **200 OK** sent with SDP
3. **ACK** received → Media playback starts
4. **BYE** received → Call terminated → Resources cleaned up

### Media Processing

The server automatically:
1. Plays greeting file when call is established
2. Handles RTP packets for supported codecs
3. Transcodes media files using FFmpeg (if enabled)
4. Manages concurrent media sessions

## Codec Support

| Codec | Payload Type | Sample Rate | Channels | Description |
|-------|-------------|-------------|----------|-------------|
| PCMU  | 0           | 8000 Hz     | 1        | G.711 μ-law |
| PCMA  | 8           | 8000 Hz     | 1        | G.711 A-law |
| Opus  | 96          | 48000 Hz    | 2        | Opus codec  |
| L16   | 97          | 44100 Hz    | 2        | Linear PCM  |

## Security Features

### IP Whitelisting
- Configurable IP address/CIDR block whitelist
- Automatic rejection of calls from non-whitelisted IPs
- Support for both IPv4 and IPv6

### DID Validation
- Configurable list of valid DIDs
- Automatic rejection with "404 Not Found" for invalid DIDs
- Support for alphanumeric DIDs

## Monitoring and Logging

### Statistics
The server provides real-time statistics:
- Active calls count
- RTP sessions information
- Media playback status
- Transcoding job status

### Logging
- Configurable log levels (debug, info, warn, error)
- Structured logging with context
- Optional file logging

## FFmpeg Integration

### Supported Conversions
- Any audio format → PCMU/PCMA/Opus/L16
- Real-time transcoding for unsupported formats
- Quality settings (high/medium/low)

### Example FFmpeg Commands
```bash
# Convert to PCMU
ffmpeg -i input.mp3 -acodec pcm_mulaw -ar 8000 -ac 1 -f mulaw output.ulaw

# Convert to Opus
ffmpeg -i input.wav -acodec libopus -ar 48000 -ac 2 -b:a 64k output.opus
```

## Development

### Project Structure
```
sip-server/
├── cmd/sip-server/          # Main application
├── internal/
│   ├── config/              # Configuration management
│   ├── sip/                 # SIP server implementation
│   ├── rtp/                 # RTP session management
│   ├── codec/               # Codec support
│   ├── media/               # Media playback
│   └── ffmpeg/              # FFmpeg integration
├── configs/                 # Configuration files
├── media-files/             # Media file storage
└── docs/                    # Documentation
```

### Adding New Codecs

1. Register the codec in `codec/codecs.go`:
```go
cm.codecs["G722"] = &SupportedCodec{
    Name:        "G722",
    PayloadType: 9,
    ClockRate:   8000,
    Channels:    1,
    // ...
}
```

2. Add processing logic in `rtp/manager.go`:
```go
case 9: // G722
    m.processG722(session, packet)
```

### Testing

```bash
# Run tests
go test ./...

# Run with race detection
go test -race ./...

# Run with coverage
go test -cover ./...
```

## Troubleshooting

### Common Issues

1. **Port binding errors**: Check if SIP port 5060 is already in use
2. **RTP port exhaustion**: Increase RTP port range or reduce call timeout
3. **FFmpeg not found**: Install FFmpeg or disable in configuration
4. **Media file not found**: Check media file paths in configuration

### Debug Mode

Enable debug logging for detailed information:
```bash
./bin/sip-server -log-level debug
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## Support

For issues and questions:
- Create an issue on GitHub
- Check the documentation
- Review the logs with debug level enabled