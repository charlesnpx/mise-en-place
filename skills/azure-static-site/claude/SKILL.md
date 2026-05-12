---
name: azure-static-site
description: Deploy, update, and restrict visibility for local static HTML sites on Azure Static Web Apps. Use when Claude needs to take an index.html or static folder and return a hosted Azure URL, reuse a previously deployed app by stored reference, configure Microsoft Entra company-only access, or explain the CLI workflow for Azure Static Web Apps.
allowed-tools:
  - Bash
  - Read
  - Write
  - Edit
  - LS
  - Grep
  - Glob
---

# Azure Static Site

Use Azure Static Web Apps for Cloudflare Pages-like static hosting in a Microsoft/Azure tenant. Prefer this over Azure Storage static website hosting when the user wants Microsoft Entra login, route authorization, future Functions APIs, preview environments, or a managed static app URL.

## Core Workflow

1. Work from a folder containing `index.html`.
2. Use `scripts/deploy_azure_static_site.py` unless the user explicitly wants manual commands.
3. Store deployment identity in `.azure-static-site.json` in the site folder. This is the reusable reference for future updates.
4. Do not store the deployment token. Fetch it at deploy time with `az staticwebapp secrets list`.
5. Require `AZURE_STATIC_SITE_RESOURCE_GROUP` to name the approved Azure resource group. Do not infer, choose, or fall back to any other resource group.
6. For updates, reuse the existing ref file or pass `--name`; the ref file resource group must match `AZURE_STATIC_SITE_RESOURCE_GROUP`.

If the user provides inline HTML, asks for a simple generated page, or does not care where the source lives, create a durable generated-site folder under `~/.cache/static-site/<ref>/`, write `index.html` into it, and deploy that folder. Keep `.azure-static-site.json` beside `index.html` so the same Azure Static Web App can be updated later. Use a project folder instead only when the user explicitly wants the source in the current repo or another named location.

## Helper Script

Deploy a new public site:

```bash
export AZURE_STATIC_SITE_RESOURCE_GROUP="<approved-resource-group>"

python3 ~/.claude/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site
```

Deploy a generated page from the durable cache:

```bash
export AZURE_STATIC_SITE_RESOURCE_GROUP="<approved-resource-group>"

ref="hello-world"
site_dir="$HOME/.cache/static-site/$ref"
mkdir -p "$site_dir"

cat > "$site_dir/index.html" <<'HTML'
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Hello World</title>
  </head>
  <body>Hello world</body>
</html>
HTML

python3 ~/.claude/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir "$site_dir"
```

Deploy or update an existing site using the stored reference:

```bash
python3 ~/.claude/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site \
  --ref-file /path/to/site/.azure-static-site.json
```

Create or update a specific app:

```bash
export AZURE_STATIC_SITE_RESOURCE_GROUP="my-static-sites-rg"

python3 ~/.claude/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site \
  --name my-static-site \
  --location eastus2
```

Only deploy to the resource group in `AZURE_STATIC_SITE_RESOURCE_GROUP`. The helper refuses `--resource-group`, refuses to create resource groups, and refuses to deploy when an existing `.azure-static-site.json` names a different resource group. If Azure returns `Forbidden` while checking the configured resource group, stop and report that the account lacks the required Azure permission. Do not infer a fallback from other visible resource groups, and do not deploy to an unrelated resource group just because the account can see or access it.

Restrict access to the user's company Entra tenant:

```bash
python3 ~/.claude/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site \
  --company-auth
```

The helper creates or updates:

- an Azure Static Web App
- a `.azure-static-site.json` reference file
- a `staticwebapp.config.json` auth file when `--company-auth` is used and no config exists
- a single-tenant Microsoft Entra app registration when `--company-auth` is used and the ref file does not already contain one

## Reference File

The ref file is the stable update handle. Keep it with the source folder when possible. For generated one-off deployments, the source folder should be `~/.cache/static-site/<ref>/`, with both `index.html` and `.azure-static-site.json` inside it.

```json
{
  "schema": 1,
  "provider": "azure-static-web-apps",
  "resourceGroup": "<approved-resource-group>",
  "name": "my-static-site",
  "location": "eastus2",
  "sku": "Free",
  "subscriptionId": "...",
  "tenantId": "...",
  "defaultHostname": "...azurestaticapps.net",
  "url": "https://...azurestaticapps.net",
  "authAppId": "..."
}
```

To update the same hosted app later, run the helper from the same folder or pass `--ref-file`. The resource group and app name are the Azure ID for deploy/update purposes.

## Company-Only Visibility

Use `--company-auth` when the user asks for same-company, organization-only, Microsoft tenant-only, Entra-only, or internal visibility.

Important details:

- `allowedRoles: ["authenticated"]` only means "logged in".
- The built-in/preconfigured auth provider is not enough for tenant-only access.
- Use a custom Microsoft Entra ID provider with a single-tenant app registration (`signInAudience=AzureADMyOrg`).
- Use the Static Web Apps Standard SKU for custom auth. The helper switches new `--company-auth` apps to `Standard`.
- If `staticwebapp.config.json` already exists, do not overwrite it. Inspect and update it carefully.

## Prerequisites

Required local tools:

```bash
az version
swa --version
```

If `swa` is missing:

```bash
npm i -g @azure/static-web-apps-cli
```

The user must be logged in:

```bash
az login
az account show
```

Required environment:

```bash
export AZURE_STATIC_SITE_RESOURCE_GROUP="<approved-resource-group>"
```

This resource group must already exist and must be approved for static site deployments. The helper does not create resource groups.

For company auth, the user also needs permission to create an Entra app registration or an admin-provided client ID and secret that can be stored as Static Web App app settings.

## Official Docs

- Azure Static Web Apps overview: https://learn.microsoft.com/azure/static-web-apps/overview
- Static Web Apps CLI: https://learn.microsoft.com/azure/static-web-apps/static-web-apps-cli
- Static Web Apps auth: https://learn.microsoft.com/azure/static-web-apps/authentication-authorization
- Custom auth provider: https://learn.microsoft.com/azure/static-web-apps/authentication-custom
- Configuration file: https://learn.microsoft.com/azure/static-web-apps/configuration
