package provider

// CodeArts' MaaS gateway (snap-access.<region>.myhuaweicloud.com) is signed
// the way Huawei's own SDKs sign: SDK-HMAC-SHA256 over a canonical request,
// with the account's AccessKey ID and Secret (the Access= form of the
// Authorization header, not the Credential= form Volcengine and Tencent use).
//
// The pieces below mirror volcSign in volcengine_usage.go -- same canonical
// shape, same sha256-hex payload hash -- because the differences are only the
// algorithm name, the header names and the key derivation: Huawei derives no
// scope, so one HMAC over the string to sign is enough.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	codeartsSignAlgo = "SDK-HMAC-SHA256"
	// codeartsEmptyBodySHA256 is sha256(""), the payload hash of a request
	// that carries no body (the session and catalog calls).
	codeartsEmptyBodySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// TakesCodeArtsAccessKey says p is on a CodeArts MaaS gateway, whose editor
// then asks for the account's AccessKey ID and Secret.
//
// The hosts are snap-access.<region>.myhuaweicloud.com for the production
// gateway of every region; the check keeps to that shape so a custom provider
// pointed elsewhere still takes a plain key.
func TakesCodeArtsAccessKey(p Provider) bool {
	if p.Account != nil {
		return false
	}
	for _, base := range []string{p.Chat, p.Responses, p.Anthropic} {
		host := strings.ToLower(hostOf(base))
		if strings.HasPrefix(host, "snap-access.") && strings.HasSuffix(host, ".myhuaweicloud.com") {
			return true
		}
	}
	return false
}

// codeartsSign signs req for a CodeArts MaaS gateway with ak/sk at now.
//
// It sets X-Sdk-Date, X-Sdk-Content-Sha256 (when there is a body), Host and
// Authorization on req. The signed header set is host plus X-Sdk-Date plus the
// content hash plus the caller's own Content-Type: CodeArts rejects a signature
// whose canonical headers do not cover a header the service read.
func codeartsSign(req *http.Request, ak, sk string, body []byte, now time.Time) {
	xDate := now.UTC().Format("20060102T150405Z")

	host := req.URL.Host
	if host == "" {
		host = req.Host
	}

	payloadHash := codeartsEmptyBodySHA256
	if len(body) > 0 {
		payloadHash = codeartsHex(sha256Sum(body))
	}

	set := map[string]string{
		"host":       host,
		"x-sdk-date": xDate,
	}
	if len(body) > 0 {
		set["x-sdk-content-sha256"] = payloadHash
	}
	if ct := req.Header.Get("Content-Type"); ct != "" {
		set["content-type"] = ct
	}

	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)

	var canonHeaders strings.Builder
	for _, n := range names {
		canonHeaders.WriteString(n)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(strings.TrimSpace(set[n]))
		canonHeaders.WriteByte('\n')
	}
	signedNames := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		codeartsCanonicalURI(req.URL.Path),
		codeartsCanonicalQuery(req.URL.RawQuery),
		canonHeaders.String(),
		signedNames,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		codeartsSignAlgo,
		xDate,
		codeartsHex(sha256Sum([]byte(canonicalRequest))),
	}, "\n")

	mac := hmac.New(sha256.New, []byte(sk))
	mac.Write([]byte(stringToSign))
	sig := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Sdk-Date", xDate)
	if len(body) > 0 {
		req.Header.Set("X-Sdk-Content-Sha256", payloadHash)
	}
	req.Header.Set("Host", host)
	req.Header.Set("Authorization", codeartsSignAlgo+" Access="+ak+
		", SignedHeaders="+signedNames+", Signature="+sig)
}

// codeartsCanonicalURI escapes each path segment and forces a trailing slash.
func codeartsCanonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = codeartsEscape(s)
	}
	joined := strings.Join(segs, "/")
	if !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined
}

// codeartsCanonicalQuery sorts by key and escapes key and value separately.
func codeartsCanonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	v, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		vals := v[k]
		sort.Strings(vals)
		for _, val := range vals {
			parts = append(parts, codeartsEscape(k)+"="+codeartsEscape(val))
		}
	}
	return strings.Join(parts, "&")
}

// codeartsEscape is Huawei's urlEscape: A-Z a-z 0-9 - _ . ~ + stay, every
// other byte becomes %XX in uppercase hex.
func codeartsEscape(s string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '+':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0f])
		}
	}
	return b.String()
}

func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}

func codeartsHex(b []byte) string { return hex.EncodeToString(b) }
