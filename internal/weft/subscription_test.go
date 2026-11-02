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
	rec := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec.Code, rec.Body.String()
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
