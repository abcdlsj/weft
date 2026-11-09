package weft

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// A backup intentionally carries configuration, not secrets: hosts and
// subscriptions keep their own credentials and are not exported.
type backupFile struct {
	Version         int             `json:"version"`
	ExportedAt      int64           `json:"exportedAt"`
	DefaultTemplate string          `json:"defaultTemplate"`
	RuleSets        []backupRuleSet `json:"ruleSets"`
	Bundles         []backupBundle  `json:"bundles"`
}

type backupRuleSet struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Source      string `json:"source"`
}

type backupBundle struct {
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	GroupBy     string         `json:"groupBy"`
	GroupType   string         `json:"groupType"`
	Template    string         `json:"template"`
	RuleSetName string         `json:"ruleSetName"`
	Members     []backupMember `json:"members"`
}

type backupMember struct {
	HostName string `json:"hostName"`
	Key      string `json:"key"`
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	sets, err := s.store.ListRuleSets()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	bundles, err := s.store.ListBundles()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	hostName := map[string]string{}
	for _, h := range hosts {
		hostName[h.ID] = h.Name
	}
	names := map[string]string{}
	out := backupFile{Version: 1, ExportedAt: now()}
	out.DefaultTemplate, _ = s.store.GetSetting("default_template")
	for _, rs := range sets {
		out.RuleSets = append(out.RuleSets, backupRuleSet{
			Name: rs.Name, Description: rs.Description, Content: rs.Content, Source: rs.Source,
		})
		names[rs.ID] = rs.Name
	}
	for _, b := range bundles {
		bb := backupBundle{
			Name: b.Name, Slug: b.Slug, GroupBy: b.GroupBy, GroupType: b.GroupType,
			Template: b.Template, RuleSetName: names[b.RuleSetID],
		}
		for _, m := range b.Members {
			bb.Members = append(bb.Members, backupMember{HostName: hostName[m.HostID], Key: m.Key})
		}
		out.Bundles = append(out.Bundles, bb)
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	filename := fmt.Sprintf("weft-backup-%s.json", time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", contentDisposition(filename))
	w.Write(append(body, '\n'))
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var body backupFile
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "备份文件不是合法 JSON")
		return
	}
	if body.Version != 1 {
		writeErr(w, 400, "不支持的备份版本")
		return
	}
	if len(body.RuleSets) > 200 || len(body.Bundles) > 200 {
		writeErr(w, 400, "备份条目过多")
		return
	}
	var warnings []string
	createdRules, updatedRules := 0, 0
	existingRules := map[string]*RuleSet{}
	ruleID := map[string]string{}
	if sets, err := s.store.ListRuleSets(); err == nil {
		for i := range sets {
			existingRules[sets[i].Name] = &sets[i]
			ruleID[sets[i].Name] = sets[i].ID
		}
	}
	for _, rs := range body.RuleSets {
		name := strings.TrimSpace(rs.Name)
		if name == "" {
			warnings = append(warnings, "跳过没有名称的规则集")
			continue
		}
		if err := ValidateRuleSetContent(rs.Content); err != nil {
			warnings = append(warnings, fmt.Sprintf("规则集 %s：%v", name, err))
			continue
		}
		if old, ok := existingRules[name]; ok {
			old.Description, old.Content, old.Source = rs.Description, rs.Content, rs.Source
			if err := s.store.UpdateRuleSet(*old); err != nil {
				warnings = append(warnings, fmt.Sprintf("规则集 %s：%v", name, err))
				continue
			}
			ruleID[name] = old.ID
			updatedRules++
		} else {
			created, err := s.store.CreateRuleSet(RuleSet{
				Name: name, Description: rs.Description, Content: rs.Content, Source: rs.Source,
			})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("规则集 %s：%v", name, err))
				continue
			}
			ruleID[name] = created.ID
			createdRules++
		}
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	hostID := map[string]string{}
	for _, h := range hosts {
		hostID[h.Name] = h.ID
	}
	existingBundles := map[string]*Bundle{}
	if bundles, err := s.store.ListBundles(); err == nil {
		for i := range bundles {
			existingBundles[bundles[i].Slug] = &bundles[i]
		}
	}
	createdBundles, updatedBundles := 0, 0
	for _, bb := range body.Bundles {
		if strings.TrimSpace(bb.Name) == "" || strings.TrimSpace(bb.Slug) == "" {
			warnings = append(warnings, "跳过没有名称或路径的编排")
			continue
		}
		var members []Member
		for _, m := range bb.Members {
			id, ok := hostID[m.HostName]
			if !ok {
				warnings = append(warnings, fmt.Sprintf("编排 %s：机器 %s 不存在，跳过该节点", bb.Name, m.HostName))
				continue
			}
			members = append(members, Member{HostID: id, Key: m.Key})
		}
		bundle := Bundle{
			Name: bb.Name, Slug: bb.Slug, GroupBy: bb.GroupBy, GroupType: bb.GroupType,
			Template: bb.Template, RuleSetID: ruleID[bb.RuleSetName],
		}
		normalizeBundle(&bundle)
		if old, ok := existingBundles[bb.Slug]; ok {
			bundle.ID, bundle.CreatedAt = old.ID, old.CreatedAt
			if err := s.store.UpdateBundle(bundle); err != nil {
				warnings = append(warnings, fmt.Sprintf("编排 %s：%v", bb.Name, err))
				continue
			}
			updatedBundles++
		} else {
			created, err := s.store.CreateBundle(bundle)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("编排 %s：%v", bb.Name, err))
				continue
			}
			bundle.ID = created.ID
			createdBundles++
		}
		if err := s.store.SetMembers(bundle.ID, members); err != nil {
			warnings = append(warnings, fmt.Sprintf("编排 %s 的节点：%v", bb.Name, err))
		}
	}
	if strings.TrimSpace(body.DefaultTemplate) != "" {
		if err := s.store.SetSetting("default_template", body.DefaultTemplate); err != nil {
			warnings = append(warnings, "默认模板："+err.Error())
		}
	}
	writeJSON(w, 200, map[string]any{
		"ruleSetsCreated": createdRules,
		"ruleSetsUpdated": updatedRules,
		"bundlesCreated":  createdBundles,
		"bundlesUpdated":  updatedBundles,
		"warnings":        warnings,
	})
}
