package weft

import (
	"crypto/ecdh"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// A bare proxy box has no mihomo to report through, but it does have the
// service configs sitting on disk. Reading them is enough to reconstruct the
// client side of every node the machine serves: only the public address has to
// be supplied by hand.

// ServiceConfigs is what `weft agent` reads off a machine that serves hy2 or
// vless directly.
type ServiceConfigs struct {
	Hysteria   string
	Xray       string
	PublicHost string
}

func (c ServiceConfigs) Empty() bool {
	return strings.TrimSpace(c.Hysteria) == "" && strings.TrimSpace(c.Xray) == ""
}

// CollectServiceNodes reads every configured service and returns ready-to-report
// nodes. One broken file does not hide the other, so failures come back as
// notes rather than an error.
func CollectServiceNodes(cfg ServiceConfigs) ([]Node, []string) {
	var nodes []Node
	var notes []string
	if path := strings.TrimSpace(cfg.Hysteria); path != "" {
		found, err := ParseHysteriaConfig(path, cfg.PublicHost)
		if err != nil {
			notes = append(notes, "hysteria: "+err.Error())
		}
		nodes = append(nodes, found...)
	}
	if path := strings.TrimSpace(cfg.Xray); path != "" {
		found, err := ParseXrayConfig(path, cfg.PublicHost)
		if err != nil {
			notes = append(notes, "xray: "+err.Error())
		}
		nodes = append(nodes, found...)
	}
	return dedupeNodes(nodes), notes
}

func dedupeNodes(nodes []Node) []Node {
	seen := map[string]bool{}
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Key == "" || seen[n.Key] {
			continue
		}
		seen[n.Key] = true
		out = append(out, n)
	}
	return out
}

// ---------- hysteria2 ----------

type hysteriaConfig struct {
	Listen string `yaml:"listen"`
	ACME   struct {
		Domains []string `yaml:"domains"`
		Email   string   `yaml:"email"`
	} `yaml:"acme"`
	TLS struct {
		Cert     string `yaml:"cert"`
		Key      string `yaml:"key"`
		SNI      string `yaml:"sni"`
		Insecure bool   `yaml:"insecure"`
	} `yaml:"tls"`
	Auth struct {
		Type     string            `yaml:"type"`
		Password string            `yaml:"password"`
		UserPass map[string]string `yaml:"userpass"`
	} `yaml:"auth"`
	Obfs struct {
		Type       string `yaml:"type"`
		Salamander struct {
			Password string `yaml:"password"`
		} `yaml:"salamander"`
	} `yaml:"obfs"`
	Bandwidth struct {
		Up   string `yaml:"up"`
		Down string `yaml:"down"`
	} `yaml:"bandwidth"`
}

func ParseHysteriaConfig(path, publicHost string) ([]Node, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg hysteriaConfig
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("%s 不是合法的 yaml：%w", filepath.Base(path), err)
	}
	port := firstPort(cfg.Listen)
	if port == 0 {
		return nil, fmt.Errorf("配置里没有可用的 listen 端口")
	}

	host := strings.TrimSpace(publicHost)
	if host == "" {
		host = "127.0.0.1"
	}

	sni, selfSigned := hysteriaSNI(cfg, filepath.Dir(path))

	creds := hysteriaCredentials(cfg)
	if len(creds) == 0 {
		return nil, fmt.Errorf("配置里没有 auth 凭据")
	}

	nodes := make([]Node, 0, len(creds))
	for i, cred := range creds {
		name := cred.name
		if name == "" {
			name = fmt.Sprintf("hy2-%d", port)
			if i > 0 {
				name = fmt.Sprintf("%s-%d", name, i+1)
			}
		}
		m := map[string]any{
			"name":     name,
			"type":     "hysteria2",
			"server":   host,
			"port":     port,
			"password": cred.password,
		}
		if sni != "" {
			m["sni"] = sni
		}
		if selfSigned || cfg.TLS.Insecure {
			m["skip-cert-verify"] = true
		}
		if strings.EqualFold(cfg.Obfs.Type, "salamander") && cfg.Obfs.Salamander.Password != "" {
			m["obfs"] = "salamander"
			m["obfs-password"] = cfg.Obfs.Salamander.Password
		}
		if cfg.Bandwidth.Up != "" {
			m["up"] = cfg.Bandwidth.Up
		}
		if cfg.Bandwidth.Down != "" {
			m["down"] = cfg.Bandwidth.Down
		}
		n, err := NodeFromProxy(m)
		if err != nil {
			return nil, err
		}
		n.Source = NodeConfig
		nodes = append(nodes, n)
	}
	return nodes, nil
}

type namedCred struct {
	name     string
	password string
}

func hysteriaCredentials(cfg hysteriaConfig) []namedCred {
	switch strings.ToLower(cfg.Auth.Type) {
	case "", "password":
		if cfg.Auth.Password == "" {
			return nil
		}
		return []namedCred{{password: cfg.Auth.Password}}
	case "userpass":
		out := make([]namedCred, 0, len(cfg.Auth.UserPass))
		for user, pass := range cfg.Auth.UserPass {
			out = append(out, namedCred{name: user, password: pass})
		}
		return out
	}
	return nil
}

// hysteriaSNI prefers an explicit sni, then the ACME domain, then the SAN of the
// certificate on disk. A self-signed certificate also means the client has to
// skip verification, which we report alongside.
func hysteriaSNI(cfg hysteriaConfig, dir string) (sni string, selfSigned bool) {
	if cfg.TLS.SNI != "" {
		return cfg.TLS.SNI, false
	}
	if len(cfg.ACME.Domains) > 0 && cfg.ACME.Domains[0] != "" {
		return cfg.ACME.Domains[0], false
	}
	if cfg.TLS.Cert == "" {
		return "", false
	}
	path := cfg.TLS.Cert
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", true
	}
	block, _ := pem.Decode(body)
	if block == nil {
		return "", true
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", true
	}
	switch {
	case len(cert.DNSNames) > 0:
		return cert.DNSNames[0], cert.IsCA || isSelfSigned(cert)
	case cert.Subject.CommonName != "":
		return cert.Subject.CommonName, isSelfSigned(cert)
	}
	return "", isSelfSigned(cert)
}

func isSelfSigned(cert *x509.Certificate) bool {
	return cert.CheckSignatureFrom(cert) == nil
}

// firstPort reads the first entry of a hysteria listen string. It may be
// ":443", "0.0.0.0:443", "[::]:443" or a port list like ":443,5000-6000".
func firstPort(listen string) int {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return 0
	}
	if i := strings.Index(listen, ","); i > 0 {
		listen = listen[:i]
	}
	if i := strings.Index(listen, "-"); i > 0 {
		listen = listen[:i]
	}
	_, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		portStr = strings.TrimPrefix(listen, ":")
	}
	if i := strings.Index(portStr, "-"); i > 0 {
		portStr = portStr[:i]
	}
	port := intOf(portStr)
	if port <= 0 || port > 65535 {
		return 0
	}
	return port
}

// ---------- xray ----------

type xrayConfig struct {
	Inbounds []struct {
		Tag      string `json:"tag"`
		Port     any    `json:"port"`
		Protocol string `json:"protocol"`
		Settings struct {
			Clients []struct {
				ID    string `json:"id"`
				Flow  string `json:"flow"`
				Email string `json:"email"`
			} `json:"clients"`
		} `json:"settings"`
		StreamSettings struct {
			Network         string `json:"network"`
			Security        string `json:"security"`
			RealitySettings struct {
				ServerNames []string `json:"serverNames"`
				PrivateKey  string   `json:"privateKey"`
				ShortIDs    []string `json:"shortIds"`
				Dest        string   `json:"dest"`
			} `json:"realitySettings"`
			TLSSettings struct {
				ServerName string   `json:"serverName"`
				Alpn       []string `json:"alpn"`
			} `json:"tlsSettings"`
		} `json:"streamSettings"`
	} `json:"inbounds"`
}

func ParseXrayConfig(path, publicHost string) ([]Node, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg xrayConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("%s 不是合法的 json：%w", filepath.Base(path), err)
	}

	host := strings.TrimSpace(publicHost)
	if host == "" {
		host = "127.0.0.1"
	}

	var nodes []Node
	for _, in := range cfg.Inbounds {
		protocol := strings.ToLower(in.Protocol)
		if protocol != "vless" && protocol != "vmess" && protocol != "trojan" {
			continue
		}
		port := intOf(in.Port)
		if port == 0 {
			break
		}
		security := strings.ToLower(in.StreamSettings.Security)
		network := in.StreamSettings.Network
		if network == "" {
			network = "tcp"
		}

		var sni string
		var reality map[string]any
		if security == "reality" {
			rs := in.StreamSettings.RealitySettings
			if len(rs.ServerNames) > 0 {
				sni = rs.ServerNames[0]
			} else if host, _, err := net.SplitHostPort(rs.Dest); err == nil {
				sni = host
			}
			pub, err := RealityPublicKey(rs.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("reality 公钥推导失败：%w", err)
			}
			reality = map[string]any{"public-key": pub}
			if len(rs.ShortIDs) > 0 && rs.ShortIDs[0] != "" {
				reality["short-id"] = rs.ShortIDs[0]
			}
		} else if security == "tls" {
			sni = in.StreamSettings.TLSSettings.ServerName
		}

		clients := in.Settings.Clients
		if len(clients) == 0 {
			continue
		}
		for i, c := range clients {
			name := in.Tag
			if name == "" {
				name = fmt.Sprintf("%s-%d", protocol, port)
			}
			if len(clients) > 1 {
				label := c.Email
				if label == "" {
					label = fmt.Sprintf("%d", i+1)
				}
				name = name + "·" + label
			}
			m := map[string]any{
				"name":    name,
				"type":    protocol,
				"server":  host,
				"port":    port,
				"network": network,
			}
			switch protocol {
			case "vless":
				m["uuid"] = c.ID
				m["flow"] = c.Flow
				m["tls"] = security == "reality" || security == "tls"
				m["servername"] = sni
				if c.Flow == "" {
					delete(m, "flow")
				}
			case "vmess":
				m["uuid"] = c.ID
				m["alterId"] = 0
				m["cipher"] = "auto"
				if security == "tls" {
					m["tls"] = true
					m["servername"] = sni
				}
			case "trojan":
				m["password"] = c.ID
				m["sni"] = sni
			}
			if reality != nil {
				m["reality-opts"] = reality
			}
			if network == "ws" {
				m["ws-opts"] = map[string]any{"path": "/"}
			}
			n, err := NodeFromProxy(m)
			if err != nil {
				return nil, err
			}
			n.Source = NodeConfig
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// RealityPublicKey turns the server's privateKey into the publicKey a client
// needs. X25519 is the whole trick: the key on disk is the raw 32-byte scalar.
func RealityPublicKey(privateKey string) (string, error) {
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return "", fmt.Errorf("配置里没有 privateKey")
	}
	raw, err := decodeBase64(privateKey)
	if err != nil {
		return "", fmt.Errorf("privateKey 不是合法 base64")
	}
	bytes := []byte(raw)
	if len(bytes) != 32 {
		return "", fmt.Errorf("privateKey 长度应为 32 字节，实际 %d", len(bytes))
	}
	key, err := ecdh.X25519().NewPrivateKey(bytes)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}
