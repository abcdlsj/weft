package weft

import (
	"encoding/base64"
	"net"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestImportGFWList(t *testing.T) {
	raw := "! comment\n||google.com\n||example.org\n@@||direct.example.com\n|https://news.example.net/path\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	content, err := ImportRuleSetContent([]byte(encoded))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"DOMAIN-SUFFIX,direct.example.com,DIRECT",
		"DOMAIN-SUFFIX,google.com,PROXY",
		"DOMAIN-SUFFIX,news.example.net,PROXY",
		"MATCH,PROXY",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("imported rules missing %q:\n%s", want, content)
		}
	}
}

func TestApplyRuleSetReplacesRulesAndAppendsGroups(t *testing.T) {
	hosts, nodes := testNodes()
	rs := &RuleSet{Name: "custom", Content: `rules:
  - DOMAIN-SUFFIX,example.com,AI
proxy-groups:
  - name: AI
    type: select
    proxies: [PROXY, DIRECT]
rule-providers:
  ads:
    type: http
    behavior: domain
    url: https://example.com/ads.yaml
    path: ./ruleset/ads.yaml
    interval: 86400
`}
	out, err := BuildMihomoYAML(Bundle{GroupBy: "host", GroupType: "url-test"}, hosts, nodes, rs)
	if err != nil {
		t.Fatal(err)
	}
	yaml := string(out)
	for _, want := range []string{
		"DOMAIN-SUFFIX,example.com,AI",
		"MATCH,PROXY",
		"name: AI",
		"rule-providers:",
		"ads:",
		"url: https://example.com/ads.yaml",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config missing %q:\n%s", want, yaml)
		}
	}
	// generated groups survive a merge
	if !strings.Contains(yaml, "name: PROXY") || !strings.Contains(yaml, "name: AUTO") {
		t.Fatalf("generated groups disappeared:\n%s", yaml)
	}
	var doc map[string]any
	if err := yamlUnmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	rules, _ := doc["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("want the custom rule plus MATCH fallback, got %#v", rules)
	}
}

func TestValidateRuleSetRejectsUnknownKeys(t *testing.T) {
	if err := ValidateRuleSetContent("dns:\n  enable: true\n"); err == nil {
		t.Fatal("unknown key should be rejected")
	}
	if err := ValidateRuleSetContent("rules:\n  - MATCH,DIRECT\n"); err != nil {
		t.Fatal(err)
	}
}

func yamlUnmarshal(data []byte, out any) error { return yaml.Unmarshal(data, out) }

func TestRuleWarnings(t *testing.T) {
	warnings := RuleWarnings(`rules:
  - RULE-SET,missing,PROXY
  - DOMAIN-SUFFIX,example.com,NoSuchGroup
  - DOMAIN-SUFFIX,ok.com,DIRECT
rule-providers:
  ads:
    type: http
    behavior: domain
    url: https://example.com/ads.yaml
    path: ./ruleset/ads.yaml
    interval: 86400
`)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "missing") || !strings.Contains(joined, "NoSuchGroup") {
		t.Fatalf("warnings missed a bad reference: %#v", warnings)
	}
	if strings.Contains(joined, "ok.com") {
		t.Fatalf("valid rule produced a warning: %#v", warnings)
	}
}

func TestDisallowedIP(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fd00::1"} {
		ip := net.ParseIP(raw)
		if !disallowedIP(ip) {
			t.Errorf("%s should be blocked", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if disallowedIP(net.ParseIP(raw)) {
			t.Errorf("%s should be allowed", raw)
		}
	}
}

func TestFetchRuleSourceRejectsLoopback(t *testing.T) {
	if _, err := FetchRuleSource("http://127.0.0.1:9/rules.txt"); err == nil {
		t.Fatal("loopback fetch should be rejected")
	}
	if _, err := FetchRuleSource("file:///etc/passwd"); err == nil {
		t.Fatal("file scheme should be rejected")
	}
}
