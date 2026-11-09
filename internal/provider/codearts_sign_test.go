package provider

// The vectors below came from a Python implementation of Huawei's
// SDK-HMAC-SHA256 that was then replayed against the live CodeArts gateway and
// answered 200: the catalog read returned 11 models, the chat completion
// returned a reply. So these are not values this Go code produced -- they are
// values the service already accepted. If the assertions pass, the port signs
// the way the gateway expects.

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

const (
	testCodeartsAK = "HPUAXQFIYV0ABBL66N1Z"
	testCodeartsSK = "1BFPBV2JpDFfiq0FGWjykhSSPBdp4inny56L4pAv"

	// GET /snap-manager/v1/statistics/plugin, signed headers
	// content-type;host;x-sdk-date, no body. Verified: HTTP 200, 11 models.
	wantCatalogAuth = "SDK-HMAC-SHA256 Access=HPUAXQFIYV0ABBL66N1Z, " +
		"SignedHeaders=content-type;host;x-sdk-date, " +
		"Signature=061c66059fa402e45e2d329778f7def0a0a73426ba2e5229bcc99660bbc3ee62"

	// POST /api/v2/chat/completions with the body below. Verified: HTTP 200
	// with a real completion.
	wantChatBody        = `{"model":"glm-5.3","messages":[{"role":"user","content":"hi"}],"stream":false}`
	wantChatPayloadHash = "4ab483fa960fa2a8f9977e9650eb66895f2bf90813bc5d5e173d7c19350bea6b"
	wantChatAuth        = "SDK-HMAC-SHA256 Access=HPUAXQFIYV0ABBL66N1Z, " +
		"SignedHeaders=content-type;host;x-sdk-content-sha256;x-sdk-date, " +
		"Signature=91d872df73e4ccd468c9977c5b5119abd3fe212c40208036072b82ff26c84bbf"
)

// TestCodeArtsSignCatalogVector signs a bodyless GET and demands byte equality
// with the Authorization header the live gateway accepted.
func TestCodeArtsSignCatalogVector(t *testing.T) {
	now, err := time.Parse("20060102T150405Z", "20261109T043716Z")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse("https://snap-access.cn-north-4.myhuaweicloud.com/snap-manager/v1/statistics/plugin")
	if err != nil {
		t.Fatal(err)
	}

	req := &http.Request{Method: "GET", URL: u, Header: http.Header{}}
	req.Header.Set("Content-Type", "application/json")

	codeartsSign(req, testCodeartsAK, testCodeartsSK, nil, now)

	if got := req.Header.Get("X-Sdk-Date"); got != "20261109T043716Z" {
		t.Errorf("X-Sdk-Date: got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != wantCatalogAuth {
		t.Fatalf("Authorization\n got: %s\nwant: %s", got, wantCatalogAuth)
	}
	// a bodyless request signs no payload hash
	if got := req.Header.Get("X-Sdk-Content-Sha256"); got != "" {
		t.Errorf("X-Sdk-Content-Sha256 should be unset on a bodyless request, got %q", got)
	}
}

// TestCodeArtsSignChatVector signs a POST carrying a JSON body: the header set
// gains x-sdk-content-sha256, so SignedHeaders order changes too.
func TestCodeArtsSignChatVector(t *testing.T) {
	now, err := time.Parse("20060102T150405Z", "20261109T044000Z")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse("https://snap-access.cn-north-4.myhuaweicloud.com/api/v2/chat/completions")
	if err != nil {
		t.Fatal(err)
	}

	req := &http.Request{Method: "POST", URL: u, Header: http.Header{}}
	req.Header.Set("Content-Type", "application/json")

	codeartsSign(req, testCodeartsAK, testCodeartsSK, []byte(wantChatBody), now)

	if got := req.Header.Get("X-Sdk-Content-Sha256"); got != wantChatPayloadHash {
		t.Fatalf("payload hash: got %s want %s", got, wantChatPayloadHash)
	}
	if got := req.Header.Get("Authorization"); got != wantChatAuth {
		t.Fatalf("Authorization\n got: %s\nwant: %s", got, wantChatAuth)
	}
}

// TestCodeArtsCanonicalURIAndQuery pins the escaping rules separately: a
// mismatch there surfaces as an opaque 401 from the service, not a readable
// diff, so it is cheaper to assert here.
func TestCodeArtsCanonicalURIAndQuery(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "/"},
		{"/", "/"},
		{"/api/v2/chat/completions", "/api/v2/chat/completions/"},
		{"/a b/c", "/a%20b/c/"},
		{"/a+b", "/a+b/"},
	}
	for _, c := range cases {
		if got := codeartsCanonicalURI(c.in); got != c.want {
			t.Errorf("canonicalURI(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	if got := codeartsCanonicalQuery("b=2&a=1"); got != "a=1&b=2" {
		t.Errorf("canonicalQuery sorted: got %q", got)
	}
	if got := codeartsCanonicalQuery("a=a b"); got != "a=a%20b" {
		t.Errorf("canonicalQuery escaped: got %q", got)
	}
	if got := codeartsCanonicalQuery(""); got != "" {
		t.Errorf("canonicalQuery empty: got %q", got)
	}
}

func TestTakesCodeArtsAccessKey(t *testing.T) {
	yes := Provider{Chat: "https://snap-access.cn-north-4.myhuaweicloud.com/api/v2"}
	no := Provider{Chat: "https://api.openai.com/v1"}
	if !TakesCodeArtsAccessKey(yes) {
		t.Error("snap-access host should take an access key")
	}
	if TakesCodeArtsAccessKey(no) {
		t.Error("api.openai.com should not take an access key")
	}
}
