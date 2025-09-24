package config

import (
	"net"
	"testing"
)

func TestConfig_IsIPAllowed(t *testing.T) {
	config := &Config{
		Security: SecurityConfig{
			IPWhitelist: []string{"127.0.0.1", "192.168.1.0/24"},
		},
	}

	// Parse the whitelist
	for _, ipStr := range config.Security.IPWhitelist {
		_, network, err := net.ParseCIDR(ipStr)
		if err != nil {
			ip := net.ParseIP(ipStr)
			if ip != nil {
				if ip.To4() != nil {
					_, network, _ = net.ParseCIDR(ipStr + "/32")
				} else {
					_, network, _ = net.ParseCIDR(ipStr + "/128")
				}
			}
		}
		if network != nil {
			config.Security.AllowedNetworks = append(config.Security.AllowedNetworks, network)
		}
	}

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"localhost", "127.0.0.1", true},
		{"private_network", "192.168.1.100", true},
		{"outside_network", "192.168.2.1", false},
		{"public_ip", "8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Invalid IP: %s", tt.ip)
			}
			result := config.IsIPAllowed(ip)
			if result != tt.expected {
				t.Errorf("IsIPAllowed(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestConfig_IsDIDValid(t *testing.T) {
	config := &Config{
		DIDs: DIDsConfig{
			ValidDIDs: []string{"1001", "1002", "support", "sales"},
		},
	}

	tests := []struct {
		name     string
		did      string
		expected bool
	}{
		{"valid_numeric", "1001", true},
		{"valid_alpha", "support", true},
		{"invalid_did", "9999", false},
		{"empty_did", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.IsDIDValid(tt.did)
			if result != tt.expected {
				t.Errorf("IsDIDValid(%s) = %v, expected %v", tt.did, result, tt.expected)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid_config",
			config: &Config{
				Server: ServerConfig{
					SIPPort: 5060,
					RTPPortRange: PortRange{
						Min: 10000,
						Max: 40000,
					},
				},
				DIDs: DIDsConfig{
					ValidDIDs: []string{"1001"},
				},
				Media: MediaConfig{
					SupportedCodecs: []string{"PCMU"},
				},
				Concurrency: ConcurrencyConfig{
					MaxConcurrentCalls: 100,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_sip_port",
			config: &Config{
				Server: ServerConfig{
					SIPPort: -1,
					RTPPortRange: PortRange{
						Min: 10000,
						Max: 40000,
					},
				},
				DIDs: DIDsConfig{
					ValidDIDs: []string{"1001"},
				},
				Media: MediaConfig{
					SupportedCodecs: []string{"PCMU"},
				},
				Concurrency: ConcurrencyConfig{
					MaxConcurrentCalls: 100,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid_port_range",
			config: &Config{
				Server: ServerConfig{
					SIPPort: 5060,
					RTPPortRange: PortRange{
						Min: 40000,
						Max: 10000,
					},
				},
				DIDs: DIDsConfig{
					ValidDIDs: []string{"1001"},
				},
				Media: MediaConfig{
					SupportedCodecs: []string{"PCMU"},
				},
				Concurrency: ConcurrencyConfig{
					MaxConcurrentCalls: 100,
				},
			},
			wantErr: true,
		},
		{
			name: "no_dids",
			config: &Config{
				Server: ServerConfig{
					SIPPort: 5060,
					RTPPortRange: PortRange{
						Min: 10000,
						Max: 40000,
					},
				},
				DIDs: DIDsConfig{
					ValidDIDs: []string{},
				},
				Media: MediaConfig{
					SupportedCodecs: []string{"PCMU"},
				},
				Concurrency: ConcurrencyConfig{
					MaxConcurrentCalls: 100,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}