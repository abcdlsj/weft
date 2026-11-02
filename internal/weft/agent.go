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

// AgentConfig is everything `weft agent` needs on a host that already runs mihomo.
type AgentConfig struct {
	Server     string
	Token      string
	Controller string
	Secret     string
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
		cfg.Interval = 15 * time.Second
	}
	client := NewMihomoClient(cfg.Controller, cfg.Secret)
	httpc := &http.Client{Timeout: 15 * time.Second}
	backoff := cfg.Interval

	for {
		start := time.Now()
		nodes, version, err := client.Collect()
		if err != nil {
			log.Printf("agent: reading mihomo at %s: %v", cfg.Controller, err)
			backoff = nextBackoff(backoff, cfg.Interval)
		} else {
			report := agentReport{
				AgentVersion:  AgentVersion,
				Controller:    cfg.Controller,
				MihomoVersion: version,
				Nodes:         nodes,
			}
			if err := postReport(httpc, cfg.Server, cfg.Token, report); err != nil {
				log.Printf("agent: reporting to %s: %v", cfg.Server, err)
				backoff = nextBackoff(backoff, cfg.Interval)
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

func nextBackoff(cur, base time.Duration) time.Duration {
	next := cur * 2
	if next > 60*time.Second {
		next = 60 * time.Second
	}
	if next < base {
		next = base
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
