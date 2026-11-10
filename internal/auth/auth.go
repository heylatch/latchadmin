// Package auth: password hashing (argon2id), TOTP two-factor codes, the one-time setup PIN and login rate limits.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// ---------- Passwords ----------

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

// HashPassword returns an encoded argon2id hash.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// CheckPassword compares pw with an encoded hash in constant time.
func CheckPassword(encoded, pw string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var th uint8
	if _, err := fmt.Sscanf(p[3], "m=%d,t=%d,p=%d", &m, &t, &th); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(p[4])
	want, err2 := base64.RawStdEncoding.DecodeString(p[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, th, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ValidPassword enforces a minimum. Long passphrases are encouraged; there are no composition rules.
func ValidPassword(pw string) error {
	if len(pw) < 10 {
		return errors.New("use at least 10 characters")
	}
	if len(pw) > 200 {
		return errors.New("use at most 200 characters")
	}
	return nil
}

// ---------- Tokens ----------

// NewToken returns a random URL-safe token.
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is how session tokens are stored.
func HashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// ---------- TOTP (RFC 6238, SHA-1, 6 digits, 30 s) ----------

// NewTOTPSecret returns a base32 secret for an authenticator app.
func NewTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

// TOTPURI is the otpauth:// link shown as a QR code.
func TOTPURI(secret, account, issuer string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		urlEscape(issuer), urlEscape(account), secret, urlEscape(issuer))
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func totpAt(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000), nil
}

// CheckTOTP accepts the code for now and one step either side.
func CheckTOTP(secret, code string) bool {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 || len(strings.TrimSpace(secret)) < 16 {
		return false
	}
	step := time.Now().Unix() / 30
	for d := int64(-1); d <= 1; d++ {
		want, err := totpAt(secret, step+d)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// ---------- Setup PIN ----------

// SetupPIN guards the first-run page: only someone who can read the panel's console can create the owner.
type SetupPIN struct {
	mu       sync.Mutex
	pin      string
	attempts int
}

// New makes a fresh 6-digit PIN.
func (s *SetupPIN) New() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	s.pin = fmt.Sprintf("%06d", n.Int64())
	s.attempts = 0
	return s.pin
}

// Check compares a PIN. After 5 wrong tries the PIN is replaced (returned as rotated, to print it again).
func (s *SetupPIN) Check(pin string) (ok bool, rotated string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pin != "" && subtle.ConstantTimeCompare([]byte(s.pin), []byte(strings.TrimSpace(pin))) == 1 {
		return true, ""
	}
	s.attempts++
	if s.attempts >= 5 {
		n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
		s.pin = fmt.Sprintf("%06d", n.Int64())
		s.attempts = 0
		return false, s.pin
	}
	return false, ""
}

// Clear ends setup.
func (s *SetupPIN) Clear() {
	s.mu.Lock()
	s.pin = ""
	s.mu.Unlock()
}

// ---------- Rate limit ----------

// Limiter counts failures per key in a sliding window.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *Limiter) prune(key string, now time.Time) []time.Time {
	h := l.hits[key]
	i := 0
	for i < len(h) && now.Sub(h[i]) > l.window {
		i++
	}
	h = h[i:]
	if len(h) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = h
	}
	return h
}

// Blocked reports whether key has used up its tries, and for how long.
func (l *Limiter) Blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	h := l.prune(key, now)
	if len(h) >= l.max {
		return true, l.window - now.Sub(h[0])
	}
	return false, 0
}

// Fail records a failure.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.hits[key] = append(l.prune(key, now), now)
}

// Reset forgets key (after a success).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}
