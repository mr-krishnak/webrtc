# Simple SIP Client Examples

## Using SIPp for Testing

### Basic Call Test

```bash
# Install SIPp
sudo apt-get install sipp

# Make a simple call to DID 1001
sipp -sn uac 192.168.1.100:5060 -s 1001

# Make call with custom scenario
sipp -sn uac 192.168.1.100:5060 -s 1001 -d 10000
```

### Load Testing

```bash
# 10 concurrent calls, 1 call per second
sipp -sn uac 192.168.1.100:5060 -s 1001 -l 10 -r 1

# Run for 60 seconds
sipp -sn uac 192.168.1.100:5060 -s 1001 -d 60000
```

## Using Asterisk

### Configuration

```ini
# /etc/asterisk/sip.conf
[sip-server]
type=peer
host=192.168.1.100
port=5060
context=outbound
disallow=all
allow=ulaw,alaw,opus
qualify=yes
```

### Dialplan

```ini
# /etc/asterisk/extensions.conf
[outbound]
exten => _1XXX,1,Dial(SIP/sip-server/${EXTEN})
exten => _1XXX,n,Hangup()
```

## Testing Different DIDs

```bash
# Test valid DIDs
sipp -sn uac 192.168.1.100:5060 -s 1001  # Should succeed
sipp -sn uac 192.168.1.100:5060 -s 1002  # Should succeed
sipp -sn uac 192.168.1.100:5060 -s support  # Should succeed

# Test invalid DID
sipp -sn uac 192.168.1.100:5060 -s 9999  # Should get 404 Not Found
```

## Testing IP Whitelisting

```bash
# From whitelisted IP (should work)
sipp -sn uac -i 192.168.1.10 192.168.1.100:5060 -s 1001

# From non-whitelisted IP (should get 403 Forbidden)
sipp -sn uac -i 10.0.0.10 192.168.1.100:5060 -s 1001
```

## Monitoring Call Statistics

```bash
# Check server logs
tail -f /path/to/sip-server.log

# Monitor network traffic
tcpdump -i any -n port 5060
tcpdump -i any -n portrange 10000-40000
```