// Package config provides configuration management for the SIP server
package config

import (
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the complete configuration for the SIP server
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Security    SecurityConfig    `yaml:"security"`
	DIDs        DIDsConfig        `yaml:"dids"`
	Media       MediaConfig       `yaml:"media"`
	FFmpeg      FFmpegConfig      `yaml:"ffmpeg"`
	Logging     LoggingConfig     `yaml:"logging"`
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
}

// ServerConfig contains server-specific configuration
type ServerConfig struct {
	SIPPort      int           `yaml:"sip_port"`
	RTPPortRange PortRange     `yaml:"rtp_port_range"`
	BindAddress  string        `yaml:"bind_address"`
}

// PortRange defines a range of ports
type PortRange struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

// SecurityConfig contains security-related configuration
type SecurityConfig struct {
	IPWhitelist []string `yaml:"ip_whitelist"`
	// Parsed CIDR blocks for efficient matching
	AllowedNetworks []*net.IPNet `yaml:"-"`
}

// DIDsConfig contains DID validation configuration
type DIDsConfig struct {
	ValidDIDs []string `yaml:"valid_dids"`
}

// MediaConfig contains media processing configuration
type MediaConfig struct {
	SupportedCodecs []string `yaml:"supported_codecs"`
	MediaFilesPath  string   `yaml:"media_files_path"`
	GreetingFile    string   `yaml:"greeting_file"`
}

// FFmpegConfig contains FFmpeg-related configuration
type FFmpegConfig struct {
	Path    string `yaml:"path"`
	Enabled bool   `yaml:"enabled"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"`
}

// ConcurrencyConfig contains concurrency limits
type ConcurrencyConfig struct {
	MaxConcurrentCalls int `yaml:"max_concurrent_calls"`
	CallTimeoutSeconds int `yaml:"call_timeout_seconds"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Parse IP whitelist into CIDR blocks
	for _, ipStr := range config.Security.IPWhitelist {
		// Try to parse as CIDR first
		_, network, err := net.ParseCIDR(ipStr)
		if err != nil {
			// If not CIDR, try as single IP
			ip := net.ParseIP(ipStr)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address or CIDR: %s", ipStr)
			}
			// Convert single IP to /32 or /128 CIDR
			if ip.To4() != nil {
				_, network, _ = net.ParseCIDR(ipStr + "/32")
			} else {
				_, network, _ = net.ParseCIDR(ipStr + "/128")
			}
		}
		config.Security.AllowedNetworks = append(config.Security.AllowedNetworks, network)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.SIPPort <= 0 || c.Server.SIPPort > 65535 {
		return fmt.Errorf("invalid SIP port: %d", c.Server.SIPPort)
	}

	if c.Server.RTPPortRange.Min <= 0 || c.Server.RTPPortRange.Max > 65535 {
		return fmt.Errorf("invalid RTP port range: %d-%d", c.Server.RTPPortRange.Min, c.Server.RTPPortRange.Max)
	}

	if c.Server.RTPPortRange.Min >= c.Server.RTPPortRange.Max {
		return fmt.Errorf("RTP port range min must be less than max: %d >= %d", c.Server.RTPPortRange.Min, c.Server.RTPPortRange.Max)
	}

	if len(c.DIDs.ValidDIDs) == 0 {
		return fmt.Errorf("no valid DIDs configured")
	}

	if len(c.Media.SupportedCodecs) == 0 {
		return fmt.Errorf("no supported codecs configured")
	}

	if c.Concurrency.MaxConcurrentCalls <= 0 {
		return fmt.Errorf("max concurrent calls must be positive: %d", c.Concurrency.MaxConcurrentCalls)
	}

	return nil
}

// IsIPAllowed checks if an IP address is in the whitelist
func (c *Config) IsIPAllowed(ip net.IP) bool {
	for _, network := range c.Security.AllowedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// IsDIDValid checks if a DID is valid
func (c *Config) IsDIDValid(did string) bool {
	for _, validDID := range c.DIDs.ValidDIDs {
		if validDID == did {
			return true
		}
	}
	return false
}