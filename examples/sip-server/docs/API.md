# SIP Server API Documentation

## Overview

This document describes the SIP Server API and integration points.

## SIP Protocol Support

### Supported Methods

- **INVITE**: Initiates a new call session
- **ACK**: Acknowledges successful call establishment
- **BYE**: Terminates an existing call
- **CANCEL**: Cancels a pending call invitation

### SIP Message Flow

#### Successful Call Flow

```
Client                    SIP Server
  |                           |
  |-------INVITE------------->|
  |<------100 Trying----------|
  |<------200 OK--------------|
  |-------ACK---------------->|
  |                           |
  |<======RTP Media==========>|
  |                           |
  |-------BYE---------------->|
  |<------200 OK--------------|
```

#### Call Rejection Flow

```
Client                    SIP Server
  |                           |
  |-------INVITE------------->|
  |<------403 Forbidden-------|  (IP not whitelisted)
  |                           |
  
  |-------INVITE------------->|
  |<------404 Not Found-------|  (Invalid DID)
  |                           |

  |-------INVITE------------->|
  |<------503 Unavailable-----|  (Server busy)
```

## Configuration API

### Configuration Structure

```yaml
server:
  sip_port: 5060           # SIP listening port
  rtp_port_range:          # RTP port allocation range
    min: 10000
    max: 40000
  bind_address: "0.0.0.0"  # Bind address

security:
  ip_whitelist:            # Allowed IP addresses/CIDR blocks
    - "127.0.0.1"
    - "192.168.1.0/24"

dids:
  valid_dids:              # Valid destination numbers
    - "1001"
    - "support"

media:
  supported_codecs:        # Supported audio codecs
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

## RTP Media Handling

### Supported Codecs

| Codec | Payload Type | Sample Rate | Bit Rate | Description |
|-------|-------------|-------------|----------|-------------|
| PCMU  | 0           | 8 kHz       | 64 kbps  | G.711 μ-law |
| PCMA  | 8           | 8 kHz       | 64 kbps  | G.711 A-law |
| Opus  | 96          | 48 kHz      | Variable | Opus codec  |
| L16   | 97          | 44.1 kHz    | Variable | Linear PCM  |

### SDP Negotiation

The server generates SDP answers with the following structure:

```sdp
v=0
o=sipserver 123456 654321 IN IP4 192.168.1.100
s=SIP Server Session
c=IN IP4 192.168.1.100
t=0 0
m=audio 20000 RTP/AVP 0 8 96
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:96 opus/48000/2
a=sendrecv
```

## Security Features

### IP Whitelisting

- Supports individual IP addresses
- Supports CIDR notation for network ranges
- IPv4 and IPv6 compatible
- Automatic rejection with 403 Forbidden

### DID Validation

- Configurable list of valid DIDs
- Alphanumeric DID support
- Automatic rejection with 404 Not Found

## Media Processing

### File Playback

The server can play media files to callers:

- Automatic greeting playback on call connect
- Support for multiple audio formats (with FFmpeg)
- Loop playback support
- Real-time transcoding

### FFmpeg Integration

Supported conversions:
- MP3 → PCMU/PCMA
- WAV → Opus
- FLAC → L16
- Any format → Telephony codecs

## Error Handling

### SIP Response Codes

| Code | Reason Phrase        | Condition |
|------|---------------------|-----------|
| 100  | Trying              | Call processing |
| 200  | OK                  | Success |
| 403  | Forbidden           | IP not whitelisted |
| 404  | Not Found           | Invalid DID |
| 405  | Method Not Allowed  | Unsupported SIP method |
| 500  | Internal Server Error | Server error |
| 503  | Service Unavailable | Server busy |

### Logging

Structured logging with context:

```json
{
  "time": "2024-01-01T12:00:00Z",
  "level": "info",
  "component": "sip-invite",
  "message": "New incoming call",
  "call_id": "abc123",
  "from": "sip:user@domain.com",
  "to": "sip:1001@server.com",
  "remote_ip": "192.168.1.10"
}
```

## Statistics and Monitoring

### Call Statistics

- Active calls count
- Total calls processed
- Call duration statistics
- Codec usage statistics

### RTP Statistics

- Active RTP sessions
- Packet counts (sent/received)
- Byte counts (sent/received)
- Port allocation usage

### System Statistics

- Memory usage
- CPU usage
- FFmpeg job statistics
- Media playback sessions

## Integration Examples

### Basic SIP Client Configuration

```javascript
// SIP.js example
const ua = new SIP.UA({
  uri: 'sip:user@192.168.1.100:5060',
  transportOptions: {
    wsServers: ['ws://192.168.1.100:5060']
  }
});

// Make a call
const session = ua.invite('sip:1001@192.168.1.100:5060');
```

### Asterisk Integration

```ini
[sip-server]
type=peer
host=192.168.1.100
port=5060
context=outbound
disallow=all
allow=ulaw,alaw,opus
```

### Testing with SIPp

```bash
# Basic INVITE test
sipp -sn uac 192.168.1.100:5060 -s 1001

# Load test with 10 concurrent calls
sipp -sn uac 192.168.1.100:5060 -s 1001 -l 10 -r 1
```

## Troubleshooting

### Common Issues

1. **Port conflicts**: Ensure SIP port 5060 is available
2. **Firewall blocking**: Open UDP ports 5060 and RTP range
3. **Codec negotiation**: Check supported codecs match client
4. **Media not flowing**: Verify RTP port range accessibility

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
./sip-server -log-level debug
```

### Network Diagnostics

```bash
# Check SIP port
netstat -un | grep 5060

# Check RTP ports
netstat -un | grep -E "1[0-9]{4}|2[0-9]{4}|3[0-9]{4}|40000"

# Test SIP connectivity
nc -u 192.168.1.100 5060
```