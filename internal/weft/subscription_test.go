package weft

import (
	"encoding/base64"
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

func TestSubscriptionTargetRocketAndCompatibility(t *testing.T) {
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	host, err := st.CreateHost("berlin-05", HostStatic)
	if err != nil {
		t.Fatal(err)
	}
	hy2Obfs := "hy2://pass123@94.159.107.211:36906?sni=www.bing.com&obfs=salamander&obfs-password=salamander-pw&insecure=1#Berlin-Hy2"
	var members []Member
	for _, link := range []string{realityLink, hy2Obfs} {
		p, err := ParseProxyLink(link)
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
		members = append(members, Member{HostID: host.ID, Key: n.Key})
	}
	b, err := st.CreateBundle(Bundle{
		Name: "全部线路", Slug: "all", GroupBy: "host", GroupType: "url-test",
		Template: DefaultTemplate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembers(b.ID, members); err != nil {
		t.Fatal(err)
	}
	sub, err := st.CreateSubscription(b.ID, "主力", "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	f := subFixture{server: NewServer(st, false), sub: sub}

	// target=rocket returns base64 share links containing Reality + Hy2 obfs params
	code, body, h := f.request(t, "/sub/"+sub.Token+"?target=rocket&key="+sub.AccessKey, "")
	if code != http.StatusOK {
		t.Fatalf("target=rocket status %d: %s", code, body)
	}
	if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("target=rocket content-type = %q", ct)
	}
	if cd := h.Get("Content-Disposition"); !strings.Contains(cd, ".txt") {
		t.Fatalf("target=rocket content-disposition = %q", cd)
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body))
	if err != nil {
		t.Fatalf("target=rocket is not valid base64: %v (%q)", err, body)
	}
	decoded := string(decodedBytes)
	for _, want := range []string{
		"security=reality",
		"fp=chrome",
		"pbk=RgDO6nczQXgj_D-VGus4e3pZHL4bIwFA7fHXocnW9QI",
		"sid=f8a64b66",
		"obfs=salamander",
		"obfs-password=salamander-pw",
		"insecure=1",
	} {
		if !strings.Contains(decoded, want) {
			t.Errorf("decoded target=rocket missing %q:\n%s", want, decoded)
		}
	}

	// Legacy target=base64 and target=text keep working identically
	codeB64, bodyB64, _ := f.request(t, "/sub/"+sub.Token+"?target=base64&key="+sub.AccessKey, "")
	if codeB64 != http.StatusOK || bodyB64 != body {
		t.Fatalf("target=base64 diverged: status=%d", codeB64)
	}
	codeText, bodyText, _ := f.request(t, "/sub/"+sub.Token+"?target=text&key="+sub.AccessKey, "")
	if codeText != http.StatusOK || bodyText != decoded {
		t.Fatalf("target=text diverged: status=%d", codeText)
	}

	// Explicit target=mihomo and default URL both serve mihomo YAML
	for _, path := range []string{
		"/sub/" + sub.Token + "?key=" + sub.AccessKey,
		"/sub/" + sub.Token + "?target=mihomo&key=" + sub.AccessKey,
	} {
		c, yml, hdr := f.request(t, path, "")
		if c != http.StatusOK || !strings.HasPrefix(hdr.Get("Content-Type"), "text/yaml") || !strings.Contains(yml, "proxies:") {
			t.Fatalf("%s did not return mihomo yaml: code=%d ct=%q", path, c, hdr.Get("Content-Type"))
		}
	}
}
