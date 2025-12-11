package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/angoo/omniarr/frontend/internal/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	CookieName = "omniarr_session"
	ContextKey = "user"
)

type User struct {
	ID      string   `json:"sub"`
	Email   string   `json:"email"`
	Groups  []string `json:"groups"`
	IsAdmin bool     `json:"is_admin"`
}

type Manager struct {
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
	adminRole    string
}

func NewManager(ctx context.Context, cfg config.AuthConfig) (*Manager, error) {
	provider, err := oidc.NewProvider(ctx, cfg.ProviderURL)
	if err != nil {
		return nil, err
	}

	oauth2Config := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &Manager{
		provider:     provider,
		oauth2Config: oauth2Config,
		verifier:     verifier,
		adminRole:    cfg.AdminRole,
	}, nil
}

func (m *Manager) AuthCodeURL(state string) string {
	return m.oauth2Config.AuthCodeURL(state)
}

func (m *Manager) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return m.oauth2Config.Exchange(ctx, code)
}

func (m *Manager) VerifyIDToken(ctx context.Context, token *oauth2.Token) (*User, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("no id_token field in oauth2 token")
	}

	idToken, err := m.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}

	// Debug: Log raw claims
	var rawClaims map[string]interface{}
	if err := idToken.Claims(&rawClaims); err != nil {
		slog.Error("Failed to parse raw claims", "error", err)
	} else {
		slog.Info("Raw ID Token Claims", "claims", rawClaims)
	}

	var claims struct {
		Sub         string   `json:"sub"`
		Email       string   `json:"email"`
		Groups      []string `json:"groups"`
		RealmAccess struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, err
	}

	// Merge groups and roles
	allGroups := append(claims.Groups, claims.RealmAccess.Roles...)

	// Also check Access Token for roles (Keycloak often puts them there)
	if token.AccessToken != "" {
		parts := strings.Split(token.AccessToken, ".")
		if len(parts) == 3 {
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err == nil {
				var accessClaims struct {
					RealmAccess struct {
						Roles []string `json:"roles"`
					} `json:"realm_access"`
				}
				if err := json.Unmarshal(payload, &accessClaims); err == nil {
					slog.Info("Found roles in Access Token", "roles", accessClaims.RealmAccess.Roles)
					allGroups = append(allGroups, accessClaims.RealmAccess.Roles...)
				}
			}
		}
	}

	user := &User{
		ID:     claims.Sub,
		Email:  claims.Email,
		Groups: allGroups,
	}

	slog.Info("Verifying user roles", "user_id", user.ID, "email", user.Email, "groups", user.Groups, "admin_role_config", m.adminRole)

	for _, group := range user.Groups {
		if group == m.adminRole {
			user.IsAdmin = true
			slog.Info("User is admin", "user_id", user.ID)
			break
		}
	}

	if !user.IsAdmin {
		slog.Info("User is NOT admin", "user_id", user.ID)
	}

	return user, nil
}

func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for login routes and static assets
		if strings.HasPrefix(r.URL.Path, "/login") || strings.HasPrefix(r.URL.Path, "/static") || strings.HasPrefix(r.URL.Path, "/auth") {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(CookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// In a real app, we would validate the session token here.
		// For this simplified version, we'll store the user info directly in the cookie (encrypted in production!).
		// Since we don't have a session store yet, we will just decode the base64 value.
		// WARNING: This is insecure for production. Use a proper session store (e.g., gorilla/sessions).
		
		data, err := base64.StdEncoding.DecodeString(cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		ctx := context.WithValue(r.Context(), ContextKey, &user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GenerateState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}