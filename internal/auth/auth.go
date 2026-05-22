package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/RedLicorice/GoGitIt/internal/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// User represents an authenticated principal. When auth is disabled,
// a synthetic local user is injected.
type User struct {
	Subject  string   `json:"sub"`
	Email    string   `json:"email,omitempty"`
	Name     string   `json:"name,omitempty"`
	Username string   `json:"username,omitempty"`
	Roles    []string `json:"roles,omitempty"`
}

type ctxKey int

const userKey ctxKey = 1

func UserFrom(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userKey).(*User)
	return u, ok
}

// Provider abstracts the auth backend so the rest of the app doesn't care
// whether we're running with Keycloak or in local-no-auth mode.
type Provider interface {
	Middleware(next http.Handler) http.Handler
	LoginHandler() http.HandlerFunc
	CallbackHandler() http.HandlerFunc
	LogoutHandler() http.HandlerFunc
	MeHandler() http.HandlerFunc
	Enabled() bool
}

// New returns a real OIDC provider when enabled, a passthrough otherwise.
func New(ctx context.Context, cfg config.AuthConfig) (Provider, error) {
	if !cfg.Enabled {
		return &passthrough{}, nil
	}

	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &oidcProvider{
		cfg:      cfg,
		oauth2:   oauth2Cfg,
		verifier: verifier,
		provider: provider,
	}, nil
}

// ---------- passthrough (auth disabled) ----------

type passthrough struct{}

func (p *passthrough) Enabled() bool { return false }

func (p *passthrough) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := &User{
			Subject:  "local",
			Username: "local",
			Name:     "Local User",
			Roles:    []string{"user"},
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (p *passthrough) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (p *passthrough) CallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (p *passthrough) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (p *passthrough) MeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		writeJSON(w, http.StatusOK, u)
	}
}

// ---------- oidcProvider (Keycloak) ----------

type oidcProvider struct {
	cfg      config.AuthConfig
	oauth2   *oauth2.Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
}

func (o *oidcProvider) Enabled() bool { return true }

const sessionCookie = "gogitit_session"
const stateCookie = "gogitit_oauth_state"

func (o *oidcProvider) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		idToken, err := o.verifier.Verify(r.Context(), c.Value)
		if err != nil {
			http.Error(w, "invalid session", http.StatusUnauthorized)
			return
		}

		var claims struct {
			Email             string   `json:"email"`
			Name              string   `json:"name"`
			PreferredUsername string   `json:"preferred_username"`
			RealmAccess       struct{ Roles []string } `json:"realm_access"`
		}
		if err := idToken.Claims(&claims); err != nil {
			http.Error(w, "claims parse error", http.StatusUnauthorized)
			return
		}

		u := &User{
			Subject:  idToken.Subject,
			Email:    claims.Email,
			Name:     claims.Name,
			Username: claims.PreferredUsername,
			Roles:    claims.RealmAccess.Roles,
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (o *oidcProvider) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := randomString(24)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     stateCookie,
			Value:    state,
			Path:     "/",
			HttpOnly: true,
			Secure:   o.cfg.CookieSecure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300,
		})
		http.Redirect(w, r, o.oauth2.AuthCodeURL(state), http.StatusFound)
	}
}

func (o *oidcProvider) CallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stateCookieVal, err := r.Cookie(stateCookie)
		if err != nil {
			http.Error(w, "missing state cookie", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("state") != stateCookieVal.Value {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		token, err := o.oauth2.Exchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, "token exchange failed", http.StatusBadRequest)
			return
		}

		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok {
			http.Error(w, "no id_token in response", http.StatusBadRequest)
			return
		}

		if _, err := o.verifier.Verify(r.Context(), rawIDToken); err != nil {
			http.Error(w, "id_token verification failed", http.StatusBadRequest)
			return
		}

		// Clear state cookie, set session cookie with the raw id_token.
		// Sessions live as long as the id_token's exp.
		http.SetCookie(w, &http.Cookie{
			Name: stateCookie, Path: "/", MaxAge: -1,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    rawIDToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   o.cfg.CookieSecure,
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().Add(12 * time.Hour),
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (o *oidcProvider) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Path: "/", MaxAge: -1,
		})
		// Optional: also redirect to Keycloak end_session_endpoint.
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (o *oidcProvider) MeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}

// ---------- helpers ----------

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		// best-effort
	}
}
