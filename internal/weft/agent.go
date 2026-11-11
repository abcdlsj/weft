package weft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// AgentVersion is set at build time: -X github.com/abcdlsj/weft/internal/weft.AgentVersion=v0.1.0
var AgentVersion = "dev"

// AgentConfig is everything `weft agent` needs on a host. A host may run mihomo,
// serve hysteria2 / xray directly, or both; any combination is fine.
type AgentConfig struct {
	Server     string
	Token      string
	Controller string
	Secret     string
	Services   ServiceConfigs
	Interval   time.Duration
	Verbose    bool
}

type agentReport struct {
	AgentVersion  string `json:"agent_version"`
	Controller    string `json:"controller"`
	MihomoVersion string `json:"mihomo_version"`
	Nodes         []Node `json:"nodes"`
}

// RunAgent pushes a proxy inventory to the server on a fixed cadence and backs
// off on failure. It never opens an inbound port.
func RunAgent(ctx context.Context, cfg AgentConfig) error {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}
	client := NewMihomoClient(cfg.Controller, cfg.Secret)
	httpc := &http.Client{Timeout: 15 * time.Second}
	backoff := cfg.Interval

	for {
		start := time.Now()
		nodes, version, err := collectEverything(cfg, client)
		if err != nil {
			log.Printf("agent: %v", err)
			backoff = nextBackoff(backoff)
		} else {
			report := agentReport{
				AgentVersion:  AgentVersion,
				Controller:    cfg.Controller,
				MihomoVersion: version,
				Nodes:         nodes,
			}
			if err := postReport(httpc, cfg.Server, cfg.Token, report); err != nil {
				log.Printf("agent: reporting to %s: %v", cfg.Server, err)
				backoff = nextBackoff(backoff)
			} else {
				backoff = cfg.Interval
				if cfg.Verbose {
					alive := 0
					for _, n := range nodes {
						if n.Alive {
							alive++
						}
					}
					log.Printf("agent: reported %d nodes (%d alive) in %s", len(nodes), alive,
						time.Since(start).Round(time.Millisecond))
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

// collectEverything merges the mihomo inventory with whatever the service
// configs on this host describe. A missing mihomo is not an error when the
// host serves hy2 or xray directly.
func collectEverything(cfg AgentConfig, client *MihomoClient) ([]Node, string, error) {
	var nodes []Node
	var version string
	var problems []string

	if cfg.Controller != "" {
		found, v, err := client.Collect()
		if err != nil {
			problems = append(problems, fmt.Sprintf("reading mihomo at %s: %v", cfg.Controller, err))
		} else {
			nodes = append(nodes, found...)
			version = v
		}
	}

	if !cfg.Services.Empty() {
		found, notes := CollectServiceNodes(cfg.Services)
		nodes = append(nodes, found...)
		problems = append(problems, notes...)
		for _, note := range notes {
			log.Printf("agent: %s", note)
		}
	}

	if len(nodes) == 0 && len(problems) > 0 {
		return nil, version, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	if cfg.Controller == "" && cfg.Services.Empty() {
		return nil, version, fmt.Errorf("没有采集源：给 --controller 或 --hysteria-config / --xray-config")
	}
	return nodes, version, nil
}

// A failed report retries soon instead of waiting out a full interval: the
// cadence is a budget for the happy path, not a delay after a blip.
func nextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > 60*time.Second {
		next = 60 * time.Second
	}
	if next < 15*time.Second {
		next = 15 * time.Second
	}
	return next
}

func postReport(c *http.Client, server, token string, report agentReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", joinURL(server, "/api/agent/report"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "weft-agent/"+AgentVersion)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("server replied %s", resp.Status)
	}
	return nil
}
