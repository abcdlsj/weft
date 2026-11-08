package weft

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type subFixture struct {
	server *Server
	sub    *Subscription
}

func newSubFixture(t *testing.T) subFixture {
	t.Helper()
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	host, err := st.CreateHost("h2nexus", HostStatic)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseProxyLink(hy2Link)
	if err != nil {
		t.Fatal(err)
	}
	n, err := NodeFromProxy(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNode(host.ID, n); err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBundle(Bundle{
		Name: "全部线路", Slug: "all", GroupBy: "host", GroupType: "url-test",
		Template: DefaultTemplate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembers(b.ID, []Member{{HostID: host.ID, Key: n.Key}}); err != nil {
		t.Fatal(err)
	}
	sub, err := st.CreateSubscription(b.ID, "主力", "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	if sub.AccessKey == "" {
		t.Fatal("subscription was created without an access key")
	}
	return subFixture{server: NewServer(st, false), sub: sub}
}

func (f subFixture) get(t *testing.T, url string) (int, string) {
	t.Helper()
	code, body, _ := f.request(t, url, "")
	return code, body
}

func (f subFixture) request(t *testing.T, url, bearer string) (int, string, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	f.server.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), rec.Header()
}

func TestSubscriptionNeedsTheKey(t *testing.T) {
	f := newSubFixture(t)

	cases := []struct {
		name string
		url  string
		want int
	}{
		{"no key", "/sub/" + f.sub.Token, http.StatusForbidden},
		{"wrong key", "/sub/" + f.sub.Token + "?key=deadbeef", http.StatusForbidden},
		{"empty key", "/sub/" + f.sub.Token + "?key=", http.StatusForbidden},
		{"right key", "/sub/" + f.sub.Token + "?key=" + f.sub.AccessKey, http.StatusOK},
		{"right key with target", "/sub/" + f.sub.Token + "?target=text&key=" + f.sub.AccessKey, http.StatusOK},
	}
	for _, tc := range cases {
		code, body := f.get(t, tc.url)
		if code != tc.want {
			t.Fatalf("%s: got %d want %d (%s)", tc.name, code, tc.want, body[:min(len(body), 80)])
		}
	}
}

func TestSubscriptionKeyStillYieldsNodes(t *testing.T) {
	f := newSubFixture(t)
	code, body := f.get(t, "/sub/"+f.sub.Token+"?key="+f.sub.AccessKey)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if !strings.Contains(body, "h2nexus/H2Nexus-Hy2") || !strings.Contains(body, "type: hysteria2") {
		t.Fatalf("subscription body lost the node:\n%s", body)
	}
}

func TestRotateRetiresBothHalves(t *testing.T) {
	f := newSubFixture(t)
	st := f.server.store
	token, key, err := st.RotateSubscription(f.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if token == f.sub.Token || key == f.sub.AccessKey {
		t.Fatal("rotation did not change both halves")
	}
	if code, _ := f.get(t, "/sub/"+f.sub.Token+"?key="+f.sub.AccessKey); code != http.StatusNotFound {
		t.Fatalf("old link should be gone, got %d", code)
	}
	if code, _ := f.get(t, "/sub/"+token+"?key="+key); code != http.StatusOK {
		t.Fatalf("new link should work, got %d", code)
	}
}

func TestSubscriptionAcceptsBearerToken(t *testing.T) {
	f := newSubFixture(t)
	code, body, _ := f.request(t, "/sub/"+f.sub.Token, f.sub.Bearer)
	if code != http.StatusOK {
		t.Fatalf("bearer should authorize, got %d: %s", code, body)
	}
	if code, _, _ := f.request(t, "/sub/"+f.sub.Token, "wrong"); code != http.StatusForbidden {
		t.Fatalf("wrong bearer should be forbidden, got %d", code)
	}
	// lower-case scheme is still the same auth scheme
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub/"+f.sub.Token, nil)
	req.Header.Set("Authorization", "bearer "+f.sub.Bearer)
	f.server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lowercase bearer scheme should work, got %d", rec.Code)
	}
}

func TestSubscriptionNamesTheProfileAndReportsHugeQuota(t *testing.T) {
	f := newSubFixture(t)
	code, _, h := f.request(t, "/sub/"+f.sub.Token+"?key="+f.sub.AccessKey, "")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	cd := h.Get("Content-Disposition")
	if !strings.Contains(cd, `filename="`) || !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("missing profile name in Content-Disposition: %q", cd)
	}
	if !strings.Contains(cd, "%E4%B8%BB%E5%8A%9B") {
		t.Fatalf("subscription label was not encoded: %q", cd)
	}
	info := h.Get("Subscription-Userinfo")
	if !strings.Contains(info, "total=109951162777600") || !strings.Contains(info, "expire=0") {
		t.Fatalf("unexpected Subscription-Userinfo: %q", info)
	}
}
