package weft

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const hysteriaYAML = `listen: :36906

tls:
  cert: /etc/hysteria/cert.pem
  key: /etc/hysteria/key.pem

auth:
  type: password
  password: berlin-secret

obfs:
  type: salamander
  salamander:
    password: obfs-pass

bandwidth:
  up: 200 mbps
  down: 1 gbps
`

const xrayJSON = `{
  "inbounds": [
    {
      "tag": "reality-tcp",
      "port": 8443,
      "protocol": "vless",
      "settings": {
        "clients": [
          { "id": "86c211bf-1693-4f6f-a061-71780d87e179", "flow": "xtls-rprx-vision" }
        ],
        "decryption": "none"
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "dest": "www.bing.com:443",
          "xver": 0,
          "serverNames": ["www.bing.com"],
          "privateKey": "%s",
          "shortIds": ["f8a64b66"]
        }
      }
    }
  ]
}`

// RFC 7748 section 6.1: the scalar and the u-coordinate it produces.
const (
	rfcPrivate = "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"
	rfcPublic  = "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a"
)

func TestRealityPublicKeyMatchesX25519(t *testing.T) {
	// xray stores the scalar raw-url-base64; RFC 7748 prints it as hex.
	priv := base64.RawURLEncoding.EncodeToString(mustHex(t, rfcPrivate))
	pub, err := RealityPublicKey(priv)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	got, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil {
		t.Fatalf("public key is not raw-url-base64: %v", err)
	}
	if want := mustHex(t, rfcPublic); string(got) != string(want) {
		t.Fatalf("public key mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestRealityPublicKeyRejectsJunk(t *testing.T) {
	for _, bad := range []string{"", "short", "!!!not-base64!!!"} {
		if _, err := RealityPublicKey(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestParseHysteriaConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(hysteriaYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	nodes, err := ParseHysteriaConfig(path, "94.159.107.211")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != "hysteria2" || n.Port != 36906 || n.Server != "94.159.107.211" {
		t.Fatalf("index columns: %+v", n)
	}
	if n.Source != NodeConfig {
		t.Fatalf("source: %q", n.Source)
	}
	var raw map[string]any
	if err := json.Unmarshal(n.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["password"] != "berlin-secret" {
		t.Fatalf("password: %v", raw["password"])
	}
	if raw["obfs"] != "salamander" || raw["obfs-password"] != "obfs-pass" {
		t.Fatalf("obfs: %v", raw)
	}
	// the cert path does not exist, which means we cannot trust it
	if raw["skip-cert-verify"] != true {
		t.Fatalf("expected skip-cert-verify for a missing cert: %v", raw)
	}
	if raw["up"] != "200 mbps" || raw["down"] != "1 gbps" {
		t.Fatalf("bandwidth: %v", raw)
	}
}

func TestParseHysteriaUserPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `listen: 0.0.0.0:443
auth:
  type: userpass
  userpass:
    alice: pass-a
    bob: pass-b
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	nodes, err := ParseHysteriaConfig(path, "203.0.113.9")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want one node per user, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Port != 443 {
			t.Fatalf("port: %d", n.Port)
		}
		if n.Name != "alice" && n.Name != "bob" {
			t.Fatalf("name should carry the user: %q", n.Name)
		}
	}
}

func TestParseXrayConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	priv := base64.RawURLEncoding.EncodeToString(mustHex(t, rfcPrivate))
	body := json.RawMessage(fmt.Sprintf(xrayJSON, priv))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	nodes, err := ParseXrayConfig(path, "94.159.107.211")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d: %+v", len(nodes), nodes)
	}
	n := nodes[0]
	if n.Type != "vless" || n.Port != 8443 || n.Name != "reality-tcp" {
		t.Fatalf("index columns: %+v", n)
	}
	var raw map[string]any
	if err := json.Unmarshal(n.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["uuid"] != "86c211bf-1693-4f6f-a061-71780d87e179" {
		t.Fatalf("uuid: %v", raw["uuid"])
	}
	if raw["flow"] != "xtls-rprx-vision" || raw["tls"] != true || raw["servername"] != "www.bing.com" {
		t.Fatalf("vless fields: %v", raw)
	}
	opts, ok := raw["reality-opts"].(map[string]any)
	if !ok || opts["short-id"] != "f8a64b66" {
		t.Fatalf("reality-opts: %v", raw)
	}
	pub, _ := base64.RawURLEncoding.DecodeString(opts["public-key"].(string))
	if string(pub) != string(mustHex(t, rfcPublic)) {
		t.Fatalf("public key was not derived from the private key")
	}
}

func TestCollectServiceNodesSurvivesOneBadFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "xray.json")
	priv := base64.RawURLEncoding.EncodeToString(mustHex(t, rfcPrivate))
	if err := os.WriteFile(good, []byte(fmt.Sprintf(xrayJSON, priv)), 0o600); err != nil {
		t.Fatal(err)
	}
	nodes, notes := CollectServiceNodes(ServiceConfigs{
		Hysteria:   filepath.Join(dir, "missing.yaml"),
		Xray:       good,
		PublicHost: "203.0.113.9",
	})
	if len(nodes) != 1 {
		t.Fatalf("want the xray node despite the broken hysteria file, got %d", len(nodes))
	}
	if len(notes) != 1 {
		t.Fatalf("want one note about the broken file, got %v", notes)
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var b byte
		for _, c := range []byte(s[i*2 : i*2+2]) {
			b <<= 4
			switch {
			case c >= '0' && c <= '9':
				b |= c - '0'
			case c >= 'a' && c <= 'f':
				b |= c - 'a' + 10
			default:
				t.Fatalf("bad hex %q", s)
			}
		}
		out[i] = b
	}
	return out
}
