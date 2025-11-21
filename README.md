## OmniArr
OmniArr is a generic, configuration-driven media management application designed to replace specialized tools (Sonarr, Radarr, Readarr, Lidarr) with a single, abstract application codebase.

Instead of hardcoding logic for specific media types ("Movies", "Books"), OmniArr operates as a **State Engine** for generic **Entities**. The definitions of what an Entity is, how to find it, and where to store it are injected at runtime via Configuration Maps.
