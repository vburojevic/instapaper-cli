package oauth1

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Token is an OAuth 1.0a token + secret.
type Token struct {
	Key    string
	Secret string
}

// Signer signs OAuth 1.0a requests using HMAC-SHA1.
// It is intentionally minimal and dependency-free.
type Signer struct {
	ConsumerKey    string
	ConsumerSecret string
	Now            func() time.Time
}

func NewSigner(consumerKey, consumerSecret string) *Signer {
	return &Signer{
		ConsumerKey:    consumerKey,
		ConsumerSecret: consumerSecret,
		Now:            time.Now,
	}
}

// AuthorizationHeader returns the full value for the HTTP "Authorization" header.
//
// - method should be "POST" or "GET" etc.
// - rawURL should be the full request URL (no need to strip query; this function will normalize it).
// - bodyParams are included in the signature base string (as required for application/x-www-form-urlencoded POSTs).
// - token is optional (nil for xAuth access token request).
func (s *Signer) AuthorizationHeader(method, rawURL string, bodyParams url.Values, token *Token) (string, error) {
	if s.ConsumerKey == "" || s.ConsumerSecret == "" {
		return "", errors.New("oauth1: missing consumer credentials")
	}
	nonce, err := randomNonce(16)
	if err != nil {
		return "", err
	}
	ts := s.Now().Unix()

	oauthParams := map[string]string{
		"oauth_consumer_key":     s.ConsumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        fmt.Sprintf("%d", ts),
		"oauth_version":          "1.0",
	}
	if token != nil && token.Key != "" {
		oauthParams["oauth_token"] = token.Key
	}

	normalizedURL, err := normalizeURL(rawURL)
	if err != nil {
		return "", err
	}

	paramString := normalizeParams(oauthParams, bodyParams)
	baseString := strings.ToUpper(method) + "&" + oauthEscape(normalizedURL) + "&" + oauthEscape(paramString)

	signingKey := oauthEscape(s.ConsumerSecret) + "&"
	if token != nil {
		signingKey += oauthEscape(token.Secret)
	}

	sig := signHMACSHA1(signingKey, baseString)
	oauthParams["oauth_signature"] = sig

	// Deterministic header ordering for easier debugging.
	keys := make([]string, 0, len(oauthParams))
	for k := range oauthParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := oauthEscape(oauthParams[k])
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", k, v))
	}
	return "OAuth " + strings.Join(parts, ", "), nil
}

type pair struct {
	k string
	v string
}

func normalizeParams(oauthParams map[string]string, bodyParams url.Values) string {
	pairs := make([]pair, 0, len(oauthParams)+len(bodyParams))
	for k, v := range oauthParams {
		pairs = append(pairs, pair{k: oauthEscape(k), v: oauthEscape(v)})
	}
	for k, vs := range bodyParams {
		for _, v := range vs {
			pairs = append(pairs, pair{k: oauthEscape(k), v: oauthEscape(v)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})

	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, p.k+"="+p.v)
	}
	return strings.Join(parts, "&")
}

func normalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("oauth1: invalid URL: %s", rawURL)
	}
	u.Fragment = ""
	u.RawQuery = ""
	// Per OAuth 1.0 normalization, scheme and host are lowercased.
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return scheme + "://" + host + path, nil
}

func signHMACSHA1(key, msg string) string {
	h := hmac.New(sha1.New, []byte(key))
	_, _ = h.Write([]byte(msg))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func randomNonce(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// oauthEscape encodes s per RFC3986 (unreserved: ALPHA / DIGIT / "-" / "." / "_" / "~").
// This is the encoding used by OAuth 1.0 signature base strings.
func oauthEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteString(fmt.Sprintf("%%%02X", c))
	}
	return b.String()
}
