// Package cloud signs in with Google and talks to the vrl-recordings upload
// service: who the account is, what its role may do, and uploading a
// finished recording. The protocol is in the vrl-recordings README.
package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Permissions the service grants through a role.
const (
	PermFish   = "fishing.run"
	PermUpload = "recordings.upload"
)

// Account is the signed-in Google account and what its role may do. An
// account nobody gave a role has an empty Role and no permissions.
type Account struct {
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

// Can reports whether the account's role grants permission.
func (a Account) Can(permission string) bool { return slices.Contains(a.Permissions, permission) }

// googleEndpoint is Google's OAuth endpoint (golang.org/x/oauth2/google
// would pull in the GCE metadata client for these two URLs).
var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// Client signs in through Google's desktop OAuth client and calls the service.
type Client struct {
	oauth oauth2.Config
	url   string
	http  *http.Client
}

// New returns a client for the service at url. The desktop client secret is
// not confidential (Google's installed-app flow); PKCE protects the code.
func New(url, clientID, clientSecret string) *Client {
	return &Client{
		oauth: oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: googleEndpoint, Scopes: []string{"openid", "email"}},
		url:   strings.TrimRight(url, "/"),
		http:  &http.Client{},
	}
}

// SignIn opens Google sign-in in the browser, waits for the redirect to a
// loopback port and returns the refresh token to keep.
func (c *Client) SignIn(ctx context.Context, open func(url string) error) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	conf := c.oauth
	conf.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	verifier, state := oauth2.GenerateVerifier(), oauth2.GenerateVerifier()

	codes := make(chan string, 1)
	failed := make(chan error, 1)
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/" || q.Get("state") != state {
			http.NotFound(w, r)
			return
		}
		if e := q.Get("error"); e != "" {
			fmt.Fprintln(w, "Sign-in was cancelled. You can close this tab.")
			select {
			case failed <- errors.New("sign-in cancelled: " + e):
			default:
			}
			return
		}
		fmt.Fprintln(w, "Signed in. You can close this tab and return to Visual Response Lab.")
		select {
		case codes <- q.Get("code"):
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		// consent makes Google return a refresh token on every sign-in.
		oauth2.SetAuthURLParam("prompt", "select_account consent"))
	if err := open(url); err != nil {
		return "", err
	}
	var code string
	select {
	case code = <-codes:
	case err := <-failed:
		return "", err
	case <-ctx.Done():
		return "", errors.New("sign-in timed out")
	}
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", err
	}
	if tok.RefreshToken == "" {
		return "", errors.New("google returned no refresh token")
	}
	return tok.RefreshToken, nil
}

// Session is a signed-in account. It is safe for concurrent use.
type Session struct {
	Account Account
	c       *Client
	tokens  oauth2.TokenSource
}

// Open starts a session from a refresh token and asks the service what the
// account may do.
func (c *Client) Open(ctx context.Context, refreshToken string) (*Session, error) {
	s := &Session{c: c, tokens: c.oauth.TokenSource(context.Background(), &oauth2.Token{RefreshToken: refreshToken})}
	req, err := s.request(ctx, http.MethodGet, "/v1/me", nil)
	if err != nil {
		return nil, err
	}
	if err := s.c.do(req, &s.Account); err != nil {
		return nil, err
	}
	return s, nil
}

// request builds a service request carrying the account's Google ID token,
// which the token source refreshes before it expires.
func (s *Session) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	tok, err := s.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("sign in again: %w", err)
	}
	id, _ := tok.Extra("id_token").(string)
	if id == "" {
		return nil, errors.New("google returned no ID token; sign in again")
	}
	req, err := http.NewRequestWithContext(ctx, method, s.c.url+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+id)
	return req, nil
}

// do sends req and decodes a JSON reply into v, or returns the service's reason.
func (c *Client) do(req *http.Request, v any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("upload service: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
