---
name: figma
description: "Read-only Figma/FigJam lookups from Figma URLs using the Figma REST API with curl. Use when a task references a Figma or FigJam link."
---

# figma

Read only.

## Auth

`FIGMA_TOKEN` environment variable holding a Figma personal access token (Figma → Settings → Security → Personal access tokens, read scopes). Check it with:

```bash
curl -s -H "X-Figma-Token: $FIGMA_TOKEN" https://api.figma.com/v1/me
```

## URLs

`https://www.figma.com/design/<KEY>/<name>?node-id=12-34` (also `/file/` and `/board/`). The API uses `:` in node IDs, so `12-34` becomes `12:34`.

## Commands

```bash
H="X-Figma-Token: $FIGMA_TOKEN"
curl -s -H "$H" "https://api.figma.com/v1/files/<KEY>/nodes?ids=12:34&depth=3"
curl -s -H "$H" "https://api.figma.com/v1/images/<KEY>?ids=12:34&format=png&scale=2"
curl -s -H "$H" "https://api.figma.com/v1/files/<KEY>/comments"
curl -s -H "$H" "https://api.figma.com/v1/files/<KEY>/meta"
curl -s -H "$H" "https://api.figma.com/v1/files/<KEY>/components"
curl -s -H "$H" "https://api.figma.com/v1/files/<KEY>/styles"
```

The images endpoint returns render URLs; download them with `curl -o` and view the PNG. Do not fetch `/v1/files/<KEY>` without `ids` on large files, because the response can be very large. Trim output with `jq`.
