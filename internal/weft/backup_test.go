package weft

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackupRoundTrip(t *testing.T) {
	s, st := newAuthServer(t)
	cookie, csrf := login(t, s)

	host, err := st.CreateHost("hk-01", HostStatic)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := st.CreateRuleSet(RuleSet{
		Name: "mine", Description: "d", Content: "rules:\n  - MATCH,PROXY\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBundle(Bundle{
		Name: "all", Slug: "all", GroupBy: "host", GroupType: "url-test",
		Template: DefaultTemplate, RuleSetID: rs.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembers(b.ID, []Member{{HostID: host.ID, Key: "k1"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backup", nil)
	req.AddCookie(cookie)
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("backup: %d %s", rec.Code, rec.Body.String())
	}
	body := append([]byte(nil), rec.Body.Bytes()...)
	var data backupFile
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.RuleSets) != 1 || len(data.Bundles) != 1 || data.Bundles[0].Members[0].HostName != "hk-01" {
		t.Fatalf("unexpected backup: %#v", data)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/backup", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(cookie)
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body.String())
	}
	var result struct {
		RuleSetsUpdated int `json:"ruleSetsUpdated"`
		BundlesUpdated  int `json:"bundlesUpdated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RuleSetsUpdated != 1 || result.BundlesUpdated != 1 {
		t.Fatalf("restore did not update in place: %#v", result)
	}
}
