package totp

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B, SHA-1 rows. The RFC uses 8 digits; our 6-digit codes
// are the last 6 of those.
var rfcSecret = []byte("12345678901234567890")

var rfcVectors = []struct {
	unix  int64
	code8 string
}{
	{59, "94287082"},
	{1111111109, "07081804"},
	{1111111111, "14050471"},
	{1234567890, "89005924"},
	{2000000000, "69279037"},
	{20000000000, "65353130"},
}

func TestRFC6238Vectors(t *testing.T) {
	for _, v := range rfcVectors {
		step := Step(time.Unix(v.unix, 0))
		if got := hotp(rfcSecret, step, 8); got != v.code8 {
			t.Errorf("t=%d: 8 digits = %s, want %s", v.unix, got, v.code8)
		}
		if got := Code(rfcSecret, step); got != v.code8[2:] {
			t.Errorf("t=%d: Code = %s, want %s", v.unix, got, v.code8[2:])
		}
	}
}

func TestValidate(t *testing.T) {
	now := time.Unix(1111111111, 0)
	cur := Step(now)

	for name, tc := range map[string]struct {
		code     string
		lastStep int64
		wantStep int64
		wantOK   bool
	}{
		"current":                {Code(rfcSecret, cur), 0, cur, true},
		"previous (30 s late)":   {Code(rfcSecret, cur-1), 0, cur - 1, true},
		"next (clock ahead)":     {Code(rfcSecret, cur+1), 0, cur + 1, true},
		"with a space":           {Code(rfcSecret, cur)[:3] + " " + Code(rfcSecret, cur)[3:], 0, cur, true},
		"two steps old":          {Code(rfcSecret, cur-2), 0, 0, false},
		"two steps ahead":        {Code(rfcSecret, cur+2), 0, 0, false},
		"already used":           {Code(rfcSecret, cur), cur, 0, false},
		"earlier than last used": {Code(rfcSecret, cur-1), cur, 0, false},
		"newer than last used":   {Code(rfcSecret, cur+1), cur, cur + 1, true},
		"wrong":                  {"000000", 0, 0, false},
		"too short":              {"12345", 0, 0, false},
		"letters":                {"12a456", 0, 0, false},
		"empty":                  {"", 0, 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "wrong" && Code(rfcSecret, cur) == "000000" {
				t.Skip("unlucky vector")
			}
			step, ok := Validate(rfcSecret, tc.code, now, tc.lastStep)
			if ok != tc.wantOK || step != tc.wantStep {
				t.Errorf("Validate(%q, last=%d) = %d, %v; want %d, %v", tc.code, tc.lastStep, step, ok, tc.wantStep, tc.wantOK)
			}
		})
	}
}

func TestNewSecret(t *testing.T) {
	a, b := NewSecret(), NewSecret()
	if len(a) != SecretSize || bytes.Equal(a, b) {
		t.Fatalf("NewSecret: len %d, equal %v", len(a), bytes.Equal(a, b))
	}
	if enc := EncodeSecret(a); len(enc) != 32 || strings.Contains(enc, "=") {
		t.Errorf("EncodeSecret = %q, want 32 base32 chars without padding", enc)
	}
}

func TestURI(t *testing.T) {
	uri := URI("MyCloud dev", "mo@example.com", rfcSecret)
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" {
		t.Errorf("scheme/host = %s/%s", u.Scheme, u.Host)
	}
	if u.Path != "/MyCloud dev:mo@example.com" {
		t.Errorf("label = %q", u.Path)
	}
	q := u.Query()
	if q.Get("secret") != EncodeSecret(rfcSecret) || q.Get("issuer") != "MyCloud dev" ||
		q.Get("digits") != "6" || q.Get("period") != "30" || q.Get("algorithm") != "SHA1" {
		t.Errorf("query = %v", q)
	}
}
