# Architecture Design Document: OmniArr Frontend

**Version:** 1.0
**Status:** Draft
**Implementation Language:** Go (Golang)
**Interactivity:** HTMX
**Styling:** Tailwind CSS
**Target Environment:** Kubernetes (Deployment)

## 1. Overview

The OmniArr Frontend is a unified web interface that aggregates multiple OmniArr backend instances into a single user experience. It serves as the entry point for users to discover, request, and manage media across different libraries (Movies, TV, Books, etc.).

It is designed to be stateless, configuration-driven, and secure, leveraging OIDC for user authentication and role management.

## 2. High-Level Architecture

```mermaid
graph TD
    User[User/Browser] -->|HTTPS| Ingress
    Ingress -->|HTTP| Frontend[OmniArr Frontend]
    
    subgraph "Authentication"
        Frontend -->|OIDC| IdP[Identity Provider]
        IdP -->|Claims| Frontend
    end

    subgraph "Backend Integration"
        Frontend -->|API Key| BackendA[OmniArr Backend (Movies)]
        Frontend -->|API Key| BackendB[OmniArr Backend (TV)]
        Frontend -->|API Key| BackendC[OmniArr Backend (Books)]
    end
```

### 2.1 Core Principles
1.  **Aggregation:** The frontend does not store media state. It queries connected backends to build the UI.
2.  **Server-Side Rendering (SSR):** Go templates render HTML. HTMX handles dynamic interactions (tabs, search results, modals) to avoid full page reloads.
3.  **Statelessness:** Session data (auth tokens) is stored in secure, HTTP-only cookies. Configuration is loaded at startup.

---

## 3. Configuration

The frontend is configured via a `frontend-config.yaml` file.

### 3.1 `frontend-config.yaml`
```yaml
server:
  port: 8080
  base_url: "https://omniarr.example.com"

auth:
  provider_url: "https://auth.example.com"
  client_id: "omniarr-frontend"
  client_secret: "secret-value"
  redirect_url: "https://omniarr.example.com/auth/callback"
  admin_group: "omniarr-admins" # Claim value to grant Admin role

backends:
  - id: "movies"
    name: "Movies"
    url: "http://omniarr-movies.default.svc.cluster.local"
    api_key: "backend-api-key-a"
    icon: "film" # FontAwesome or similar icon name
  
  - id: "tv"
    name: "TV Shows"
    url: "http://omniarr-tv.default.svc.cluster.local"
    api_key: "backend-api-key-b"
    icon: "tv"
```

---

## 4. User Interface & Experience

The UI is divided into two main views based on the user's role derived from OIDC claims.

### 4.1 Layout
*   **Navigation Bar:** Displays the application logo, user profile/logout, and a tab bar for switching between connected backends (e.g., "Movies", "TV").
*   **Content Area:** Renders the active tab's content.

### 4.2 User View
*   **Goal:** Discover and Request content.
*   **Tab Interface:**
    *   **Search Bar:** Prominent input field to search for new content.
        *   *Action:* Triggers `GET /catalog/lookup` on the active backend.
        *   *Display:* Grid of results (Posters/Covers).
        *   *Interaction:* "Request" button on items.
    *   **Dashboard:** (Optional) Show "Recently Added" or "Popular" items from that backend.

### 4.3 Admin View
*   **Goal:** Manage requests and system health.
*   **Tab Interface:**
    *   **Search:** Same as User View.
    *   **Library Management:** List of all tracked entities in this backend.
        *   *Actions:* Delete, Force Search, Edit Quality Profile.
    *   **Request History:** A table showing who requested what and when.
        *   *Source:* Queries the backend's extended API for request history.

---

## 5. Application Components (Go)

### 5.1 Router & Middleware
*   **Framework:** `net/http` (Standard Lib) or `Chi`/`Echo`.
*   **Auth Middleware:** Intercepts requests, validates OIDC session cookies. Redirects to IdP if missing. Injects `User` context (ID, Email, Roles) into the request.
*   **Proxy/Client:** A generic HTTP client wrapper to communicate with backends, attaching the correct API Key.

### 5.2 Handlers
*   **Auth Handlers:** `/login`, `/callback`, `/logout`.
*   **View Handlers:**
    *   `GET /`: Renders the main shell and the default tab.
    *   `GET /view/{backend_id}`: Renders the content for a specific backend tab.
*   **Action Handlers (HTMX Targets):**
    *   `POST /api/{backend_id}/search`: Proxies search to backend, returns HTML grid of results.
    *   `POST /api/{backend_id}/request`: Sends create request to backend, including `requested_by` user info.
    *   `DELETE /api/{backend_id}/entity/{id}`: Admin only. Proxies delete to backend.

### 5.3 Template Engine
*   Uses Go `html/template`.
*   **Components:** Reusable fragments for "Movie Card", "Table Row", "Modal".
*   **Tailwind:** Utility classes embedded directly in templates.

---

## 6. Backend Integration Protocol

The frontend expects every configured backend to adhere to the standard OmniArr API.

*   **Discovery:** `GET /system/config` (To get root entity name, e.g., "Movie").
*   **Search:** `GET /catalog/lookup?query=...`
*   **Request:** `POST /entities`
    *   *Payload Extension:* The frontend will append `X-Requested-By` header or include `requested_by` in the JSON body to track the user.

## 7. Security
*   **CSRF Protection:** Essential since we are using cookies and forms.
*   **Input Sanitization:** All user input (search queries) is sanitized before sending to backends.
*   **Role Enforcement:** Admin-only routes (DELETE, History View) must strictly check the `IsAdmin` flag in the session.