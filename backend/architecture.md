Here is the complete architectural design document for **OmniArr**.

***

# Architecture Design Document: OmniArr

**Version:** 1.0  
**Status:** Draft  
**Implementation Language:** Go (Golang)  
**Target Environment:** Kubernetes (StatefulSet)

## 1. Overview
OmniArr is a generic, configuration-driven media management application designed to replace specialized tools (Sonarr, Radarr, Readarr, Lidarr) with a single, abstract application codebase.

Instead of hardcoding logic for specific media types ("Movies", "Books"), OmniArr operates as a **State Engine** for generic **Entities**. The definitions of what an Entity is, how to find it, and where to store it are injected at runtime via Configuration Maps.

## 2. High-Level Architecture

### 2.1 Cluster Layout
OmniArr is designed to run as multiple distinct instances within a Kubernetes cluster, orchestrated by a separate unified frontend.

```mermaid
graph TD
    User[User/Browser] --> Ingress
    Ingress --> Frontend[Unified Frontend w/ OIDC]
    
    subgraph K8s Cluster
        Frontend -- API Key --> InstanceA[OmniArr (Movies)]
        Frontend -- API Key --> InstanceB[OmniArr (TV)]
        Frontend -- API Key --> InstanceC[OmniArr (Audiobooks)]
        
        InstanceA --> Prowlarr[Indexer Proxy]
        InstanceA --> DownloadClient[SabNZBD/NZBGet]
        InstanceA --> DB[(PostgreSQL)]
        InstanceA --> MediaPVC[Media Volume]
    end
```

### 2.2 Core Principles
1.  **Stateless Application Logic:** The container requires no local persistent storage for application data. All state is stored in an external SQL database.
2.  **Configuration Driven:** All business logic (Hierarchy, API mapping, Naming conventions) is defined in YAML ConfigMaps.
3.  **Generic Abstraction:** The Go code does not know what a "Season" is. It only understands `root_entities` and `child_entities`.
4.  **Search Index Pattern:** Metadata is stored as a JSON blob, but searchable attributes are extracted into a key-value SQL lookup table.

---

## 3. Data Layer (PostgreSQL)

To support generic media types while maintaining query performance and database compatibility, we utilize the **Search Index Pattern**.

### 3.1 Schema Definition

**Table: `entities`**
The core storage of items.
```sql
CREATE TABLE entities (
    uuid UUID PRIMARY KEY,
    parent_uuid UUID NULL REFERENCES entities(uuid), -- Handles Series->Season hierarchy
    entity_type VARCHAR(50) NOT NULL,                -- Matches config (e.g., "series", "episode")
    status VARCHAR(20) NOT NULL,                     -- WANTED, AVAILABLE, DOWNLOADED, MISSING
    monitored BOOLEAN DEFAULT TRUE,
    last_refreshed_at TIMESTAMP,                     -- For TTL Reconciliation loop
    quality_profile_id INT,
    local_path TEXT,                                 -- Location on Media PVC
    metadata JSONB                                   -- Full blob from Metadata Provider (TMDB/TVDB)
);
```

**Table: `idx_identifiers`**
Lookup table for finding entities without scanning JSONB.
```sql
CREATE TABLE idx_identifiers (
    entity_uuid UUID REFERENCES entities(uuid) ON DELETE CASCADE,
    key VARCHAR(50) NOT NULL,   -- e.g., "imdb_id", "isbn", "slug"
    value VARCHAR(255) NOT NULL,-- e.g., "tt12345", "978-3-16-148410-0"
    PRIMARY KEY (entity_uuid, key)
);
CREATE INDEX idx_lookup ON idx_identifiers(key, value);
```

---

## 4. Configuration Ecosystem (ConfigMaps)

The application behavior is dictated by four core YAML configurations mounted into the container.

### 4.1 `schema.yaml`
Defines the relationship structure and internal types.
```yaml
root_entity: "series"
entities:
  - type: "series"
    is_leaf: false
    children: ["season"]
  - type: "season"
    children: ["episode"]
  - type: "episode"
    is_leaf: true
    has_files: true
```

### 4.2 `catalog.yaml`
Defines how to talk to the implementation-specific metadata source (TMDB, TVDB, OpenLibrary) and map the response to the DB.
```yaml
provider: "TheMovieDB"
base_url: "https://api.themoviedb.org/3"
endpoints:
  - entity_type: "series"
    url: "/tv/{id}"
    # JSONPath mapping -> Internal Entity Metadata
    attributes:
      Title: "$.name"
      Year: "$.first_air_date[:4]"
      Overview: "$.overview"
    # Fields to extract into `idx_identifiers` table
    identifiers:
      - key: "tvdb_id"
        source: "$.ids.tvdb"
```

### 4.3 `quality.yaml`
Defines how to score releases.
```yaml
profiles:
  - name: "HD-1080p"
    cutoff: "1080p" # Stop upgrading once this is met
    items:
      - name: "1080p"
        score: 1000
      - name: "720p"
        score: 500
definitions:
  - name: "1080p"
    regex: "(1080p|1920x1080)"
```

### 4.4 `acquisition.yaml`
Defines search strings, parsing logic, and file naming.
```yaml
search_query_format: "{Title} S{Season:02d}E{Episode:02d}"
naming:
  folder: "/tv/{Series.Title} ({Series.Year})"
  file: "{Series.Title} - S{Season:02d}E{Episode:02d} - {Title} - {Quality}.{Ext}"
download_client:
  type: "nzbget"
  category: "tv"
```

---

## 5. Application Components (Go Implementation)

The application is a single binary composed of several Managers.

### 5.1 The Lifecycle Reconciler
*   **Role:** Keeps metadata in sync.
*   **Logic:**
    1.  Poller checks `entities` where `now() - last_refreshed_at > config.TTL`.
    2.  Fetches fresh JSON from `catalog.yaml` endpoints.
    3.  Updates `metadata` column.
    4.  **Tree Sync:** If schema defines children (e.g., Seasons), it compares API response vs DB rows. New children are inserted with status `WANTED`.

### 5.2 Search Coordinator
*   **Role:** Finds content for `WANTED` items.
*   **Logic:**
    1.  Generates query string based on `acquisition.yaml`.
    2.  Queries Prowlarr (via Torznab standard).
    3.  **Parsing:** Matches results against `quality.yaml` regex to assign a Score.
    4.  Sends best candidate to Download Client.
    5.  Records `DownloadClientID` in DB for tracking.

### 5.3 Import Manager
*   **Role:** Moving files from Downloads -> Library.
*   **Logic:**
    1.  Watches Download Client API for "Completed" status.
    2.  Matches `DownloadClientID` to the Entity.
    3.  **Atomic Operation:**
        *   Renames file based on Token String (e.g., `{Title} - {Year}`).
        *   Moves file from Download PVC to Library PVC.
        *   Updates Entity status to `DOWNLOADED`.

---

## 6. API Surface

The Backend exposes a REST API secured by an API Key. This API supports the "Unified Frontend".

### 6.1 Capabilities (Frontend Discovery)
Allows the generic UI to know what this instance supports.
*   `GET /system/config`
    *   Returns: Root Entity Name ("Movie"), Quality Profiles, Root Folders.

### 6.2 Catalog (Discovery)
Allows the UI to search for *new* content to add.
*   `GET /catalog/lookup?query=The+Matrix`
    *   Proxies request to Metadata Provider (TMDB).
    *   Returns normalized JSON + `exists_in_db` flag.

### 6.3 Management
*   `POST /entities` - Add a new item (Set status to Monitored).
*   `DELETE /entities/{uuid}` - Remove item.

### 6.4 Admin / Manual Actions
*   `POST /acquisition/search/{uuid}` - Force interactive search (returns raw candidates).
*   `POST /queue/import` - Force manual import of a file.

---

## 7. Deployment Strategy

*   **Kind:** `StatefulSet`
*   **Replicas:** `1` (Required to prevent DB lock contention and race conditions on file moves).
*   **Volumes:**
    *   `config-volume` (ConfigMap mount).
    *   `media-volume` (PVC RWX or RWO).
*   **Lifecycle Hooks:**
    *   `PreStop`: Must ensure current file copy/move operations finish before terminating the pod to prevent file corruption.

## 8. Development Roadmap (Go)

1.  **Phase 1 (Core):** Struct definitions, ConfigMap parsing (YAML -> Structs), Postgres connection, CRUD operations for Entities.
2.  **Phase 2 (Brain):** Metadata Client (HTTP client with JSONPath extraction), Reconciler Loop.
3.  **Phase 3 (Hands):** Usenet Client Implementation, Parsing logic (Regex scoring), Torznab Client.
4.  **Phase 4 (API):** Gin/Echo HTTP server implementation for frontend Endpoints.
5.  **Phase 5 (Frontend integration):** Build the separate OIDC-enabled UI.