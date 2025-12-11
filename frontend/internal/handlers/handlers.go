package handlers

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/angoo/omniarr/frontend/internal/auth"
	"github.com/angoo/omniarr/frontend/internal/config"
	"github.com/angoo/omniarr/frontend/internal/proxy"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	authManager *auth.Manager
	proxyClient *proxy.Client
	templates   *template.Template
	config      *config.Config
}

func NewHandler(authManager *auth.Manager, proxyClient *proxy.Client, cfg *config.Config) (*Handler, error) {
	tmpl, err := template.ParseGlob("internal/templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		authManager: authManager,
		proxyClient: proxyClient,
		templates:   tmpl,
		config:      cfg,
	}, nil
}

// --- Auth Handlers ---

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login.html", map[string]interface{}{
		"Title": "Login",
	})
}

func (h *Handler) LoginOIDC(w http.ResponseWriter, r *http.Request) {
	state, err := auth.GenerateState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	
	// In a real app, store state in a secure cookie to validate on callback
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		Path:     "/",
		Expires:  time.Now().Add(10 * time.Minute),
	})

	http.Redirect(w, r, h.authManager.AuthCodeURL(state), http.StatusFound)
}

func (h *Handler) AuthCallback(w http.ResponseWriter, r *http.Request) {
	// Check for error from the provider
	if errStr := r.URL.Query().Get("error"); errStr != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Error("Auth callback error", "error", errStr, "description", errDesc)
		http.Error(w, "Auth error: "+errStr+" - "+errDesc, http.StatusBadRequest)
		return
	}

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Error(w, "State cookie missing", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "State mismatch", http.StatusBadRequest)
		return
	}

	oauth2Token, err := h.authManager.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := h.authManager.VerifyIDToken(r.Context(), oauth2Token)
	if err != nil {
		http.Error(w, "Failed to verify ID token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("User logged in", "claims", user, "is_admin", user.IsAdmin)

	// Create session cookie (simplified)
	userData, _ := json.Marshal(user)
	encodedUser := base64.StdEncoding.EncodeToString(userData)

	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    encodedUser,
		HttpOnly: true,
		Secure:   false, // Set to true in production
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		HttpOnly: true,
		Path:     "/",
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// --- View Handlers ---

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	// Default to first backend if available
	if len(h.config.Backends) > 0 {
		http.Redirect(w, r, "/view/"+h.config.Backends[0].ID, http.StatusFound)
		return
	}
	
	// Fallback if no backends configured
	h.render(w, "home.html", h.baseData(r, "Home", nil))
}

func (h *Handler) ViewBackend(w http.ResponseWriter, r *http.Request) {
	backendID := chi.URLParam(r, "backendID")
	backend, ok := h.proxyClient.GetBackend(backendID)
	if !ok {
		http.Error(w, "Backend not found", http.StatusNotFound)
		return
	}

	lists, err := h.proxyClient.GetLists(backendID)
	if err != nil {
		// Log error but continue, maybe show empty lists
		slog.Error("Failed to fetch lists", "error", err)
	} else {
		slog.Info("Fetched lists", "count", len(lists))
		for _, l := range lists {
			slog.Info("List", "title", l.Title, "children_count", len(l.Children))
		}
	}

	data := h.baseData(r, backend.Name, map[string]interface{}{
		"ActiveBackend":     backendID,
		"ActiveBackendName": backend.Name,
		"ActiveBackendID":   backendID,
		"Lists":             lists,
	})

	h.render(w, "dashboard.html", data)
}

func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(auth.ContextKey).(*auth.User)
	if !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// For now, just use the first backend or allow selecting one.
	// Let's default to the first one for simplicity, or iterate all?
	// The requirement says "manage tracked entities", implying a global view or per-backend.
	// Given the current architecture, let's pick the first backend ID if available.
	if len(h.config.Backends) == 0 {
		http.Error(w, "No backends configured", http.StatusInternalServerError)
		return
	}
	backendID := h.config.Backends[0].ID

	entities, err := h.proxyClient.GetEntities(backendID)
	if err != nil {
		slog.Error("Failed to fetch entities", "error", err)
		http.Error(w, "Failed to fetch entities", http.StatusInternalServerError)
		return
	}

	data := h.baseData(r, "Admin", map[string]interface{}{
		"Entities":        entities,
		"ActiveBackendID": backendID,
	})

	h.render(w, "admin.html", data)
}

func (h *Handler) DeleteEntity(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(auth.ContextKey).(*auth.User)
	if !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	backendID := chi.URLParam(r, "backendID")
	uuid := chi.URLParam(r, "uuid")

	if err := h.proxyClient.DeleteEntity(backendID, uuid); err != nil {
		slog.Error("Failed to delete entity", "error", err)
		http.Error(w, "Failed to delete entity", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- HTMX Action Handlers ---

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	backendID := chi.URLParam(r, "backendID")
	query := r.FormValue("query")

	if query == "" {
		return // Return nothing if query is empty
	}

	results, err := h.proxyClient.Search(backendID, query)
	if err != nil {
		// Log error and return empty/error partial
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Results":   results,
		"BackendID": backendID,
	}

	if err := h.templates.ExecuteTemplate(w, "results.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) SearchPage(w http.ResponseWriter, r *http.Request) {
	backendID := chi.URLParam(r, "backendID")
	query := r.URL.Query().Get("query")

	backend, ok := h.proxyClient.GetBackend(backendID)
	if !ok {
		http.Error(w, "Backend not found", http.StatusNotFound)
		return
	}

	var results []proxy.SearchResult
	var err error

	if query != "" {
		results, err = h.proxyClient.Search(backendID, query)
		if err != nil {
			slog.Error("Search failed", "error", err)
			// Continue with empty results or show error?
			// For now, let's just log it.
		}
	}

	data := h.baseData(r, "Search Results", map[string]interface{}{
		"Results":           results,
		"Query":             query,
		"ActiveBackendID":   backendID,
		"ActiveBackendName": backend.Name,
		"ActiveBackend":     backendID, // For nav highlighting
	})

	h.render(w, "search.html", data)
}

func (h *Handler) GetEntityDetails(w http.ResponseWriter, r *http.Request) {
	backendID := chi.URLParam(r, "backendID")
	id := r.URL.Query().Get("id")
	entityType := r.URL.Query().Get("type")
	if entityType == "" {
		entityType = "book" // Default
	}

	meta, err := h.proxyClient.GetCatalogItem(backendID, entityType, id)
	if err != nil {
		http.Error(w, "Failed to fetch details: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Metadata":  meta,
		"BackendID": backendID,
	}

	h.render(w, "modal.html", data)
}

func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	backendID := chi.URLParam(r, "backendID")
	title := r.FormValue("title")
	id := r.FormValue("id")
	
	user := r.Context().Value(auth.ContextKey).(*auth.User)

	childOverrides := make(map[string]bool)
	if err := r.ParseForm(); err == nil {
		for k, v := range r.Form {
			if strings.HasPrefix(k, "child_override_") {
				childID := strings.TrimPrefix(k, "child_override_")
				childOverrides[childID] = v[0] == "on"
			}
		}
	}

	payload := proxy.RequestPayload{
		Title:          title,
		ID:             id,
		ChildOverrides: childOverrides,
	}

	err := h.proxyClient.Request(backendID, payload, user.Email)
	if err != nil {
		w.Write([]byte("<button class='bg-red-600 text-white font-bold py-2 px-4 rounded text-sm' disabled>Error</button>"))
		return
	}

	w.Write([]byte("<button class='bg-green-600 text-white font-bold py-2 px-4 rounded text-sm' disabled>Requested</button>"))
}

// --- Helpers ---

func (h *Handler) render(w http.ResponseWriter, tmplName string, data interface{}) {
	// If it's a full page render (not HTMX partial), wrap in base
	// Ideally we'd check headers or use a layout func, but for now we assume
	// login.html and dashboard.html are content blocks that need base.html wrapper
	// EXCEPT login.html which might be standalone or wrapped differently.
	// Actually, base.html defines "content" block.
	
	// Simplified: Always execute base, which executes the specific template as "content"
	// But we need to know WHICH template to use as content.
	// Standard Go template inheritance pattern:
	// Parse base + specific view.
	
	// Since we parsed glob, all templates are in h.templates.
	// We need to execute "base.html" but somehow tell it to use "dashboard.html" content.
	// The standard way is defining {{ define "content" }} in dashboard.html.
	// So if we execute "base.html", and we have parsed dashboard.html which defines "content",
	// it *should* work if we constructed the template set correctly for this request.
	// BUT, "content" is redefined by multiple files.
	// So we need to clone the template and parse the specific file, OR use different block names.
	
	// Better approach for this simple app:
	// Parse base.html + the specific view file on every request (dev mode) or cache combinations.
	// For now, let's just use the glob and assume we are careful.
	// Wait, if multiple files define "content", the last one wins in the glob.
	// So we MUST parse per request or have distinct block names.
	
	// Let's fix NewHandler to NOT parse glob, but parse base.html.
	// And render helper parses the specific view.
	
	t, err := template.ParseFiles("internal/templates/base.html", "internal/templates/"+tmplName)
	if tmplName == "results.html" || tmplName == "modal.html" {
		// Partial only
		t, err = template.ParseFiles("internal/templates/" + tmplName)
	}
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if tmplName == "results.html" || tmplName == "modal.html" {
		err = t.Execute(w, data)
	} else {
		err = t.ExecuteTemplate(w, "base.html", data)
	}
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) baseData(r *http.Request, title string, extra map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"Title":    title,
		"Backends": h.config.Backends,
		"User":     r.Context().Value(auth.ContextKey),
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}