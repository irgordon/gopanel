package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	loginContextLifetime = 15 * time.Minute
	loginContextDomain   = "gopanel-login-context-v1"
	loginTokenDomain     = "gopanel-csrf-v1-login"
	authTokenDomain      = "gopanel-csrf-v1-authenticated"
	CSRFField            = "csrf_token"
)

type CSRF struct{ key []byte }

func NewCSRFKey() ([]byte, error) { key := make([]byte, 32); _, err := rand.Read(key); return key, err }
func NewCSRF(key []byte) *CSRF    { return &CSRF{key: append([]byte(nil), key...)} }

func (csrf *CSRF) NewLoginContext(now time.Time) (string, string, time.Time, error) {
	contextValue := make([]byte, 32)
	if _, err := rand.Read(contextValue); err != nil {
		return "", "", time.Time{}, err
	}
	expires := now.Add(loginContextLifetime).UTC()
	encodedContext := base64.RawURLEncoding.EncodeToString(contextValue)
	exp := strconv.FormatInt(expires.Unix(), 10)
	mac := csrf.mac(loginContextDomain, []byte(encodedContext), []byte(exp))
	cookie := "v1." + encodedContext + "." + exp + "." + base64.RawURLEncoding.EncodeToString(mac)
	token, err := csrf.newToken(loginTokenDomain, cookie, exp)
	return cookie, token, expires, err
}

func (csrf *CSRF) LoginToken(contextCookie string, now time.Time) (string, error) {
	_, exp, err := csrf.validateLoginContext(contextCookie, now)
	if err != nil {
		return "", err
	}
	return csrf.newToken(loginTokenDomain, contextCookie, strconv.FormatInt(exp.Unix(), 10))
}
func (csrf *CSRF) ValidateLogin(contextCookie, token string, now time.Time) bool {
	_, exp, err := csrf.validateLoginContext(contextCookie, now)
	if err != nil {
		return false
	}
	return csrf.validateToken(loginTokenDomain, contextCookie, strconv.FormatInt(exp.Unix(), 10), token)
}
func (csrf *CSRF) AuthToken(session string) (string, error) {
	return csrf.newToken(authTokenDomain, session, "")
}
func (csrf *CSRF) ValidateAuth(session, token string) bool {
	return session != "" && csrf.validateToken(authTokenDomain, session, "", token)
}

func (csrf *CSRF) validateLoginContext(value string, now time.Time) ([]byte, time.Time, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 4 || parts[0] != "v1" {
		return nil, time.Time{}, errors.New("invalid login context")
	}
	contextValue, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(contextValue) != 32 {
		return nil, time.Time{}, errors.New("invalid login context")
	}
	expUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, time.Time{}, errors.New("invalid login context")
	}
	expires := time.Unix(expUnix, 0).UTC()
	if !expires.After(now) {
		return nil, time.Time{}, errors.New("expired login context")
	}
	provided, err := base64.RawURLEncoding.Strict().DecodeString(parts[3])
	if err != nil || len(provided) != sha256.Size {
		return nil, time.Time{}, errors.New("invalid login context")
	}
	expected := csrf.mac(loginContextDomain, []byte(parts[1]), []byte(parts[2]))
	if !hmac.Equal(provided, expected) {
		return nil, time.Time{}, errors.New("invalid login context")
	}
	return contextValue, expires, nil
}

func (csrf *CSRF) newToken(domain, contextValue, expiration string) (string, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(nonce)
	mac := csrf.mac(domain, []byte(contextValue), []byte(encoded), []byte(expiration))
	return "v1." + encoded + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}
func (csrf *CSRF) validateToken(domain, contextValue, expiration, token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return false
	}
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(nonce) != 32 {
		return false
	}
	provided, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	expected := csrf.mac(domain, []byte(contextValue), []byte(parts[1]), []byte(expiration))
	return hmac.Equal(provided, expected)
}

func (csrf *CSRF) mac(domain string, fields ...[]byte) []byte {
	hash := hmac.New(sha256.New, csrf.key)
	writeFrame(hash, []byte("v1"))
	writeFrame(hash, []byte(domain))
	for _, field := range fields {
		writeFrame(hash, field)
	}
	return hash.Sum(nil)
}

type writer interface{ Write([]byte) (int, error) }

func writeFrame(target writer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	target.Write(length[:])
	target.Write(value)
}
