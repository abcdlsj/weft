package weft

import "testing"

func digestOf(t *testing.T, report agentReport) string {
	t.Helper()
	digest, err := reportDigest(report)
	if err != nil {
		t.Fatalf("reportDigest: %v", err)
	}
	return digest
}

func TestReportDigestIgnoresVolatileFields(t *testing.T) {
	base := agentReport{
		AgentVersion: "v0.6.0",
		Nodes: []Node{
			{Key: "a", Name: "hy2", Type: "hysteria2", Port: 443, Alive: true, DelayMS: 12},
			{Key: "b", Name: "reality", Type: "vless", Port: 8443, Alive: false, DelayMS: 300},
		},
	}
	changed := base
	changed.Nodes = []Node{
		{Key: "a", Name: "hy2", Type: "hysteria2", Port: 443, Alive: false, DelayMS: 0},
		{Key: "b", Name: "reality", Type: "vless", Port: 8443, Alive: true, DelayMS: 41},
	}
	if digestOf(t, base) != digestOf(t, changed) {
		t.Fatal("latency and liveness must not change the digest")
	}
}

func TestReportDigestIgnoresNodeOrder(t *testing.T) {
	forward := agentReport{Nodes: []Node{{Key: "a", Port: 443}, {Key: "b", Port: 8443}}}
	reversed := agentReport{Nodes: []Node{{Key: "b", Port: 8443}, {Key: "a", Port: 443}}}
	if digestOf(t, forward) != digestOf(t, reversed) {
		t.Fatal("a differently ordered inventory must hash the same")
	}
}

func TestReportDigestSeesRealChanges(t *testing.T) {
	before := agentReport{Nodes: []Node{{Key: "a", Port: 443, Raw: []byte(`{"password":"old"}`)}}}
	after := agentReport{Nodes: []Node{{Key: "a", Port: 443, Raw: []byte(`{"password":"new"}`)}}}
	if digestOf(t, before) == digestOf(t, after) {
		t.Fatal("a changed password must change the digest")
	}
	if digestOf(t, before) == digestOf(t, agentReport{Nodes: []Node{{Key: "a", Port: 8443}}}) {
		t.Fatal("a changed port must change the digest")
	}
}
