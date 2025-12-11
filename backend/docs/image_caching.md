# Image Caching Configuration

OmniArr supports caching images for entities locally. This allows the frontend to display images even if the external metadata provider is unavailable or to reduce API calls.

## Configuration

To enable image caching for a specific entity type, you need to map the `Image` attribute in your catalog configuration file (e.g., `catalog.yaml`).

### 1. Server Configuration

Ensure your `server.yaml` (or `config/server.yaml`) has the `image_storage_path` configured. If not set, it defaults to `./images`.

```yaml
port: 8085
api_key: "your-api-key"
image_storage_path: "./images"
```

### 2. Catalog Configuration

In your catalog configuration (e.g., `books/catalog.yaml`), add the `Image` key to the `attributes` section of the entity definition. The value should be a JSONPath expression that points to the image URL in the metadata provider's response.

**Example (Google Books):**

```yaml
provider: "GoogleBooks"
base_url: "https://www.googleapis.com/books/v1"
endpoints:
  - entity_type: "book"
    url: "/volumes/{id}"
    attributes:
      Title: "$.volumeInfo.title"
      Authors: "$.volumeInfo.authors"
      Year: "$.volumeInfo.publishedDate[:4]"
      Description: "$.volumeInfo.description"
      Image: "$.volumeInfo.imageLinks.thumbnail" # <--- Add this line
```

**Example (Generic):**

```yaml
    attributes:
      Title: "$.name"
      Image: "$.images.poster" # JSONPath to the image URL
```

## How it Works

1.  **Metadata Fetching:** When OmniArr fetches metadata for an entity (during import or refresh), it extracts the `Image` URL using the configured JSONPath.
2.  **Downloading:** If an image URL is found, the backend downloads the image and stores it in the configured `image_storage_path`. The filename is generated as `{entity_type}_{id}.{ext}`.
3.  **Storage:** The local path (filename) is stored in the `entities` table in the database.
4.  **Serving:** The backend serves these images via the `/api/images/` endpoint.
5.  **Frontend:** The frontend can display these images using the URL provided by the backend.