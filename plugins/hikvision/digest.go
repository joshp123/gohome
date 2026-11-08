package hikvision

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// digestChallenge is the camera's WWW-Authenticate: Digest header (RFC 7616, MD5, qop=auth).
type digestChallenge struct {
	realm, nonce, opaque, qop, algorithm string
}

func parseDigestChallenge(header string) (digestChallenge, error) {
	scheme, rest, _ := strings.Cut(strings.TrimSpace(header), " ")
	if !strings.EqualFold(scheme, "Digest") {
		return digestChallenge{}, fmt.Errorf("camera did not offer Digest authentication: %q", scheme)
	}
	params := map[string]string{}
	for _, part := range splitDigestParams(rest) {
		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		params[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	challenge := digestChallenge{
		realm: params["realm"], nonce: params["nonce"], opaque: params["opaque"],
		qop: params["qop"], algorithm: params["algorithm"],
	}
	if challenge.nonce == "" {
		return digestChallenge{}, fmt.Errorf("digest challenge has no nonce")
	}
	if challenge.algorithm != "" && !strings.EqualFold(challenge.algorithm, "MD5") {
		return digestChallenge{}, fmt.Errorf("unsupported digest algorithm %q", challenge.algorithm)
	}
	if challenge.qop != "" && !containsToken(challenge.qop, "auth") {
		return digestChallenge{}, fmt.Errorf("unsupported digest qop %q", challenge.qop)
	}
	return challenge, nil
}

// splitDigestParams splits on commas that are outside quoted values.
func splitDigestParams(s string) []string {
	var parts []string
	var current strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			current.WriteRune(r)
		case r == ',' && !quoted:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	return append(parts, current.String())
}

func containsToken(list, token string) bool {
	for _, item := range strings.Split(list, ",") {
		if strings.TrimSpace(item) == token {
			return true
		}
	}
	return false
}

// authorization answers a fresh challenge once, so the nonce count is always 1.
func (c digestChallenge) authorization(username, password, method, uri string) string {
	ha1 := md5Hex(username + ":" + c.realm + ":" + password)
	ha2 := md5Hex(method + ":" + uri)
	fields := []string{
		fmt.Sprintf(`username="%s"`, username),
		fmt.Sprintf(`realm="%s"`, c.realm),
		fmt.Sprintf(`nonce="%s"`, c.nonce),
		fmt.Sprintf(`uri="%s"`, uri),
		`algorithm=MD5`,
	}
	if c.qop == "" {
		fields = append(fields, fmt.Sprintf(`response="%s"`, md5Hex(ha1+":"+c.nonce+":"+ha2)))
	} else {
		const nonceCount = "00000001"
		cnonce := randomHex(8)
		fields = append(fields,
			fmt.Sprintf(`response="%s"`, md5Hex(ha1+":"+c.nonce+":"+nonceCount+":"+cnonce+":auth:"+ha2)),
			`qop=auth`, "nc="+nonceCount, fmt.Sprintf(`cnonce="%s"`, cnonce))
	}
	if c.opaque != "" {
		fields = append(fields, fmt.Sprintf(`opaque="%s"`, c.opaque))
	}
	return "Digest " + strings.Join(fields, ", ")
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
