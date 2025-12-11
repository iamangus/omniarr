# Metadata Provider Standards

This document outlines the requirements and standards for implementing a Metadata Provider in OmniArr. A Metadata Provider is responsible for fetching information about media entities (Books, Movies, TV Shows, etc.) from external sources.

## Core Responsibilities

A complete Metadata Provider should support the following core capabilities:

### 1. Search with Filtering

Providers must support searching for entities. Beyond simple keyword matching, robust providers should support filtering to narrow down results.

**Requirements:**
- **Keyword Search**: Ability to search by title, author, or ISBN/ID.
- **Filtering**: Support for structured filters such as:
    - `year`: Filter by release year.
    - `type`: Filter by media type (e.g., `book`, `movie`, `series`).
    - `author` / `creator`: Filter by specific creators.
    - `genre`: Filter by genre tags.

### 2. Lists (Discovery)

To aid in discovery, providers should be able to return curated lists of content. This allows the frontend to display "browse" views.

**Standard List Types:**
- **Popular**: Items currently popular or trending.
- **Top Rated**: Items with high ratings.
- **New Releases**: Recently published or released items.
- **Upcoming**: Items scheduled for future release.

### 3. Hierarchical Metadata (Parent/Child Relationships)

Many media types are hierarchical. The provider must be able to traverse and return data for an entity and its children.

**Examples:**
- **TV Shows**: Show -> Seasons -> Episodes
- **Book Series**: Series -> Books
- **Music**: Artist -> Albums -> Tracks

**Requirements:**
- **Full Traversal**: When requesting a parent entity (e.g., a TV Show), the provider should ideally be able to return (or allow fetching of) its children (Seasons/Episodes).
- **Child Lookup**: Ability to find a specific child entity directly if supported by the upstream API.

### 4. Entity Details

The most basic function is retrieving detailed metadata for a single entity using its unique identifier.

**Standard Fields:**
- **ID**: Unique identifier from the source.
- **Title**: Display title.
- **Description**: Plot summary or synopsis.
- **Date**: Release/Publication date.
- **Creators**: Authors, Directors, etc.
- **Images**: Cover art, posters, backdrops.
- **Identifiers**: External IDs (ISBN, IMDB ID, TMDB ID) for cross-referencing.

---

## Implementation Guidelines

When implementing a new provider (e.g., in `backend/internal/providers/metadata/myprovider/`), ensure it adheres to the `MetadataProvider` interface defined in `backend/internal/metadata/provider.go`.

*Note: If the current Go interface does not yet support specific features like Lists or advanced Filtering, please document the limitations in the provider's specific documentation and implement what is currently possible.*