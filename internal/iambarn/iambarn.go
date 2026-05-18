package iambarn

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Provider implements the IAMBarn OIDC authorization-code + PKCE flow.
type Provider struct {
	issuer      string
	clientID    string
	redirectURI string

	mu          sync.RWMutex
	jwksKeys    map[string]crypto.PublicKey // *rsa.PublicKey or ed25519.PublicKey
	jwksFetched time.Time

	client *http.Client
}

// New creates a Provider. issuer is the base URL (e.g. https://iam.wiebe.xyz).
func New(issuer, clientID, redirectURI string) *Provider {
	return &Provider{
		issuer:      strings.TrimRight(issuer, "/"),
		clientID:    clientID,
		redirectURI: redirectURI,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

// GenerateVerifier generates a PKCE code_verifier (43 base64url chars from 32 random bytes).
func GenerateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Challenge derives the PKCE code_challenge (S256 method) from a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizationURL builds the redirect URL for the authorization code + PKCE flow.
func (p *Provider) AuthorizationURL(state, challenge string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.clientID},
		"redirect_uri":          {p.redirectURI},
		"scope":                 {"openid profile email offline_access"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return p.issuer + "/oauth2/authorize?" + q.Encode()
}

// Claims holds the verified identity from an ID token.
type Claims struct {
	Sub               string
	Email             string
	EmailVerified     bool
	PreferredUsername string
	Name              string
	OrganizationID    string
}

// DisplayName returns the best available human-readable identifier.
func (c *Claims) DisplayName() string {
	if c.PreferredUsername != "" {
		return c.PreferredUsername
	}
	if c.Email != "" {
		return c.Email
	}
	return c.Sub
}

// ExchangeAndValidate exchanges the authorization code for tokens and returns
// the verified identity claims from the ID token.
func (p *Provider) ExchangeAndValidate(ctx context.Context, code, verifier string) (*Claims, error) {
	idToken, err := p.exchangeCode(ctx, code, verifier)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	claims, err := p.validateIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("validate id token: %w", err)
	}
	return claims, nil
}

func (p *Provider) exchangeCode(ctx context.Context, code, verifier string) (string, error) {
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {p.clientID},
		"code":          {code},
		"redirect_uri":  {p.redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.issuer+"/oauth2/token", strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("token endpoint %d: %s", resp.StatusCode, raw)
	}

	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.IDToken == "" {
		return "", fmt.Errorf("no id_token in token response")
	}
	return tokenResp.IDToken, nil
}

func (p *Provider) validateIDToken(ctx context.Context, token string) (*Claims, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed jwt: expected 3 parts, got %d", len(parts))
	}

	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode jwt header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return nil, fmt.Errorf("parse jwt header: %w", err)
	}

	switch header.Alg {
	case "RS256", "EdDSA":
		// supported
	default:
		return nil, fmt.Errorf("unsupported jwt algorithm: %s", header.Alg)
	}

	pub, err := p.getJWKSKey(ctx, header.Kid)
	if err != nil {
		return nil, fmt.Errorf("get signing key: %w", err)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode jwt signature: %w", err)
	}
	msg := parts[0] + "." + parts[1]

	switch header.Alg {
	case "RS256":
		rsaKey, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("key type mismatch for RS256")
		}
		digest := sha256.Sum256([]byte(msg))
		if err := rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, digest[:], sig); err != nil {
			return nil, fmt.Errorf("jwt signature invalid: %w", err)
		}
	case "EdDSA":
		edKey, ok := pub.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("key type mismatch for EdDSA")
		}
		if !ed25519.Verify(edKey, []byte(msg), sig) {
			return nil, fmt.Errorf("jwt signature invalid")
		}
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode jwt payload: %w", err)
	}
	var raw struct {
		Sub               string `json:"sub"`
		Iss               string `json:"iss"`
		Aud               any    `json:"aud"` // string or []string
		Exp               int64  `json:"exp"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		OrganizationID    string `json:"organization_id"`
	}
	if err := json.Unmarshal(payloadRaw, &raw); err != nil {
		return nil, fmt.Errorf("parse jwt payload: %w", err)
	}

	if raw.Sub == "" {
		return nil, fmt.Errorf("missing sub claim")
	}
	if raw.Iss != p.issuer {
		return nil, fmt.Errorf("invalid issuer %q", raw.Iss)
	}
	if raw.Exp <= time.Now().Unix() {
		return nil, fmt.Errorf("token expired")
	}
	if !audContains(raw.Aud, p.clientID) {
		return nil, fmt.Errorf("token audience does not include client_id")
	}

	return &Claims{
		Sub:               raw.Sub,
		Email:             raw.Email,
		EmailVerified:     raw.EmailVerified,
		PreferredUsername: raw.PreferredUsername,
		Name:              raw.Name,
		OrganizationID:    raw.OrganizationID,
	}, nil
}

func audContains(aud any, clientID string) bool {
	switch v := aud.(type) {
	case string:
		return v == clientID
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == clientID {
				return true
			}
		}
	}
	return false
}

// --------------------------------------------------------------------------
// JWKS key caching
// --------------------------------------------------------------------------

type jwksJSON struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		// RSA fields
		N string `json:"n"`
		E string `json:"e"`
		// OKP (Ed25519) fields
		Crv string `json:"crv"`
		X   string `json:"x"`
	} `json:"keys"`
}

func (p *Provider) getJWKSKey(ctx context.Context, kid string) (crypto.PublicKey, error) {
	p.mu.RLock()
	key, cached := p.jwksKeys[kid]
	fresh := time.Since(p.jwksFetched) < time.Hour
	p.mu.RUnlock()

	if cached && fresh {
		return key, nil
	}

	if err := p.fetchJWKS(ctx); err != nil {
		return nil, err
	}

	p.mu.RLock()
	key, cached = p.jwksKeys[kid]
	p.mu.RUnlock()

	if !cached {
		return nil, fmt.Errorf("key %q not found in JWKS", kid)
	}
	return key, nil
}

func (p *Provider) fetchJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.issuer+"/oauth2/jwks.json", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks jwksJSON
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]crypto.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kid == "" {
			continue
		}
		switch k.Kty {
		case "RSA":
			if k.N == "" || k.E == "" {
				continue
			}
			nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				continue
			}
			eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				continue
			}
			n := new(big.Int).SetBytes(nBytes)
			e := int(new(big.Int).SetBytes(eBytes).Int64())
			keys[k.Kid] = &rsa.PublicKey{N: n, E: e}
		case "OKP":
			if k.Crv != "Ed25519" || k.X == "" {
				continue
			}
			xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
			if err != nil {
				continue
			}
			if len(xBytes) != ed25519.PublicKeySize {
				continue
			}
			keys[k.Kid] = ed25519.PublicKey(xBytes)
		}
	}

	p.mu.Lock()
	p.jwksKeys = keys
	p.jwksFetched = time.Now()
	p.mu.Unlock()
	return nil
}
