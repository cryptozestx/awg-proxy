package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type AWGConfig struct {
	Interface InterfaceConfig
	Peers     []PeerConfig
}

type InterfaceConfig struct {
	PrivateKey string
	Address    []string
	DNS        []string
	MTU        int
	Jc         int
	Jmin       int
	Jmax       int
	S1         int
	S2         int
	S3         int
	S4         int
	H1         string
	H2         string
	H3         string
	H4         string
	I1         string
	I2         string
	I3         string
	I4         string
	I5         string
}

type PeerConfig struct {
	PublicKey           string
	PresharedKey        string
	Endpoint            string
	AllowedIPs          []string
	PersistentKeepalive int
}

func base64ToHex(b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return "", nil
	}
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 key: %v", err)
	}
	if len(dec) != 32 {
		return "", fmt.Errorf("invalid key length: expected 32 bytes, got %d", len(dec))
	}
	return hex.EncodeToString(dec), nil
}

func ParseConfig(path string) (*AWGConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := &AWGConfig{
		Interface: InterfaceConfig{},
		Peers:     []PeerConfig{},
	}

	scanner := bufio.NewScanner(file)
	currentSection := ""
	var currentPeer *PeerConfig

	for scanner.Scan() {
		line := scanner.Text()
		// Remove comments and trim spaces
		if idx := strings.IndexAny(line, "#;"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.Trim(line, "[]"))
			currentSection = section
			if section == "peer" {
				if currentPeer != nil {
					config.Peers = append(config.Peers, *currentPeer)
				}
				currentPeer = &PeerConfig{}
			}
			continue
		}

		// Key-value pair
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		switch currentSection {
		case "interface":
			switch key {
			case "privatekey":
				config.Interface.PrivateKey = val
			case "address":
				ips := strings.Split(val, ",")
				for _, ip := range ips {
					config.Interface.Address = append(config.Interface.Address, strings.TrimSpace(ip))
				}
			case "dns":
				dnsList := strings.Split(val, ",")
				for _, dns := range dnsList {
					config.Interface.DNS = append(config.Interface.DNS, strings.TrimSpace(dns))
				}
			case "mtu":
				if m, err := strconv.Atoi(val); err == nil {
					config.Interface.MTU = m
				}
			case "jc":
				if v, err := strconv.Atoi(val); err == nil {
					config.Interface.Jc = v
				}
			case "jmin":
				if v, err := strconv.Atoi(val); err == nil {
					config.Interface.Jmin = v
				}
			case "jmax":
				if v, err := strconv.Atoi(val); err == nil {
					config.Interface.Jmax = v
				}
			case "s1":
				if v, err := strconv.Atoi(val); err == nil {
					config.Interface.S1 = v
				}
			case "s2":
				if v, err := strconv.Atoi(val); err == nil {
					config.Interface.S2 = v
				}
			case "s3":
				if v, err := strconv.Atoi(val); err == nil {
					config.Interface.S3 = v
				}
			case "s4":
				if v, err := strconv.Atoi(val); err == nil {
					config.Interface.S4 = v
				}
			case "h1":
				config.Interface.H1 = val
			case "h2":
				config.Interface.H2 = val
			case "h3":
				config.Interface.H3 = val
			case "h4":
				config.Interface.H4 = val
			case "i1":
				config.Interface.I1 = val
			case "i2":
				config.Interface.I2 = val
			case "i3":
				config.Interface.I3 = val
			case "i4":
				config.Interface.I4 = val
			case "i5":
				config.Interface.I5 = val
			}

		case "peer":
			if currentPeer == nil {
				currentPeer = &PeerConfig{}
			}
			switch key {
			case "publickey":
				currentPeer.PublicKey = val
			case "presharedkey":
				currentPeer.PresharedKey = val
			case "endpoint":
				currentPeer.Endpoint = val
			case "allowedips":
				ips := strings.Split(val, ",")
				for _, ip := range ips {
					currentPeer.AllowedIPs = append(currentPeer.AllowedIPs, strings.TrimSpace(ip))
				}
			case "persistentkeepalive":
				if k, err := strconv.Atoi(val); err == nil {
					currentPeer.PersistentKeepalive = k
				}
			}
		}
	}

	if currentPeer != nil {
		config.Peers = append(config.Peers, *currentPeer)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if config.Interface.PrivateKey == "" {
		return nil, fmt.Errorf("interface PrivateKey is missing")
	}

	return config, nil
}

func (c *AWGConfig) ToUAPI() (string, error) {
	var sb strings.Builder

	// Parse PrivateKey
	privHex, err := base64ToHex(c.Interface.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("invalid interface PrivateKey: %v", err)
	}
	sb.WriteString(fmt.Sprintf("private_key=%s\n", privHex))

	// Write AWG obfuscation parameters if set
	if c.Interface.Jc > 0 {
		sb.WriteString(fmt.Sprintf("jc=%d\n", c.Interface.Jc))
	}
	if c.Interface.Jmin > 0 {
		sb.WriteString(fmt.Sprintf("jmin=%d\n", c.Interface.Jmin))
	}
	if c.Interface.Jmax > 0 {
		sb.WriteString(fmt.Sprintf("jmax=%d\n", c.Interface.Jmax))
	}
	if c.Interface.S1 > 0 {
		sb.WriteString(fmt.Sprintf("s1=%d\n", c.Interface.S1))
	}
	if c.Interface.S2 > 0 {
		sb.WriteString(fmt.Sprintf("s2=%d\n", c.Interface.S2))
	}
	if c.Interface.S3 > 0 {
		sb.WriteString(fmt.Sprintf("s3=%d\n", c.Interface.S3))
	}
	if c.Interface.S4 > 0 {
		sb.WriteString(fmt.Sprintf("s4=%d\n", c.Interface.S4))
	}
	if c.Interface.H1 != "" {
		sb.WriteString(fmt.Sprintf("h1=%s\n", c.Interface.H1))
	}
	if c.Interface.H2 != "" {
		sb.WriteString(fmt.Sprintf("h2=%s\n", c.Interface.H2))
	}
	if c.Interface.H3 != "" {
		sb.WriteString(fmt.Sprintf("h3=%s\n", c.Interface.H3))
	}
	if c.Interface.H4 != "" {
		sb.WriteString(fmt.Sprintf("h4=%s\n", c.Interface.H4))
	}
	if c.Interface.I1 != "" {
		sb.WriteString(fmt.Sprintf("i1=%s\n", c.Interface.I1))
	}
	if c.Interface.I2 != "" {
		sb.WriteString(fmt.Sprintf("i2=%s\n", c.Interface.I2))
	}
	if c.Interface.I3 != "" {
		sb.WriteString(fmt.Sprintf("i3=%s\n", c.Interface.I3))
	}
	if c.Interface.I4 != "" {
		sb.WriteString(fmt.Sprintf("i4=%s\n", c.Interface.I4))
	}
	if c.Interface.I5 != "" {
		sb.WriteString(fmt.Sprintf("i5=%s\n", c.Interface.I5))
	}

	// Tell the interface to replace existing peers before appending new ones
	sb.WriteString("replace_peers=true\n")

	// Write Peers
	for _, peer := range c.Peers {
		pubHex, err := base64ToHex(peer.PublicKey)
		if err != nil {
			return "", fmt.Errorf("invalid peer PublicKey: %v", err)
		}
		sb.WriteString(fmt.Sprintf("public_key=%s\n", pubHex))

		if peer.PresharedKey != "" {
			pskHex, err := base64ToHex(peer.PresharedKey)
			if err != nil {
				return "", fmt.Errorf("invalid peer PresharedKey: %v", err)
			}
			sb.WriteString(fmt.Sprintf("preshared_key=%s\n", pskHex))
		}

		if peer.Endpoint != "" {
			sb.WriteString(fmt.Sprintf("endpoint=%s\n", peer.Endpoint))
		}

		for _, ip := range peer.AllowedIPs {
			sb.WriteString(fmt.Sprintf("allowed_ip=%s\n", ip))
		}

		if peer.PersistentKeepalive > 0 {
			sb.WriteString(fmt.Sprintf("persistent_keepalive_interval=%d\n", peer.PersistentKeepalive))
		}
	}

	return sb.String(), nil
}
