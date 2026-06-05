---
name: azure-static-site
description: Deploy, update, and restrict visibility for local static HTML sites on Azure Static Web Apps. Use when Codex needs to take an index.html or static folder and return a hosted Azure URL, reuse a previously deployed app by stored reference, configure Microsoft Entra company-only access, or explain the CLI workflow for Azure Static Web Apps.
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
7. Deployments are company-only by default using single-tenant Microsoft Entra auth. Pass `--public` only when public anonymous access is explicitly requested.

If the user provides inline HTML, asks for a simple generated page, or does not care where the source lives, create a durable generated-site folder under `~/.cache/static-site/<ref>/`, write `index.html` into it, and deploy that folder. Keep `.azure-static-site.json` beside `index.html` so the same Azure Static Web App can be updated later. Use a project folder instead only when the user explicitly wants the source in the current repo or another named location.

## Helper Script

Deploy a new company-only site:

```bash
export AZURE_STATIC_SITE_RESOURCE_GROUP="<approved-resource-group>"

python3 ~/.codex/skills/azure-static-site/scripts/deploy_azure_static_site.py \
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

python3 ~/.codex/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir "$site_dir"
```

Deploy or update an existing site using the stored reference:

```bash
python3 ~/.codex/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site \
  --ref-file /path/to/site/.azure-static-site.json
```

Create or update a specific app:

```bash
export AZURE_STATIC_SITE_RESOURCE_GROUP="my-static-sites-rg"

python3 ~/.codex/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site \
  --name my-static-site \
  --location eastus2
```

Deploy a public site only when explicitly requested:

```bash
python3 ~/.codex/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site \
  --public
```

Only deploy to the resource group in `AZURE_STATIC_SITE_RESOURCE_GROUP`. The helper refuses `--resource-group`, refuses to create resource groups, and refuses to deploy when an existing `.azure-static-site.json` names a different resource group. If Azure returns `Forbidden` while checking the configured resource group, stop and report that the account lacks the required Azure permission. Do not infer a fallback from other visible resource groups, and do not deploy to an unrelated resource group just because the account can see or access it.

The `--company-auth` flag is accepted for compatibility but is no longer required:

```bash
python3 ~/.codex/skills/azure-static-site/scripts/deploy_azure_static_site.py \
  --dir /path/to/site \
  --company-auth
```

The helper creates or updates:

- an Azure Static Web App
- a `.azure-static-site.json` reference file
- a `staticwebapp.config.json` auth file by default when no config exists
- a single-tenant Microsoft Entra app registration by default when the ref file does not already contain one

When `--public` is used, the helper skips auth file and app registration creation. It does not remove an existing `staticwebapp.config.json`; inspect the site folder if changing an existing protected site to public access.

## Reference File

The ref file is the stable update handle. Keep it with the source folder when possible. For generated one-off deployments, the source folder should be `~/.cache/static-site/<ref>/`, with both `index.html` and `.azure-static-site.json` inside it.

```json
{
  "schema": 1,
  "provider": "azure-static-web-apps",
  "resourceGroup": "<approved-resource-group>",
  "name": "my-static-site",
  "location": "eastus2",
  "sku": "Standard",
  "subscriptionId": "...",
  "tenantId": "...",
  "defaultHostname": "...azurestaticapps.net",
  "url": "https://...azurestaticapps.net",
  "authAppId": "..."
}
```

To update the same hosted app later, run the helper from the same folder or pass `--ref-file`. The resource group and app name are the Azure ID for deploy/update purposes.

## Company-Only Visibility

Company-only visibility is the default. Use it when the user asks for same-company, organization-only, Microsoft tenant-only, Entra-only, or internal visibility. Use `--public` only when the user explicitly asks for anonymous public access.

Important details:

- `allowedRoles: ["authenticated"]` only means "logged in".
- The built-in/preconfigured auth provider is not enough for tenant-only access.
- Use a custom Microsoft Entra ID provider with a single-tenant app registration (`signInAudience=AzureADMyOrg`).
- Use the Static Web Apps Standard SKU for custom auth. The helper defaults new protected apps to `Standard`.
- If `staticwebapp.config.json` already exists, do not overwrite it. Inspect and update it carefully.

Minimal auth config shape:

```json
{
  "routes": [
    {
      "route": "/*",
      "allowedRoles": ["authenticated"]
    }
  ],
  "responseOverrides": {
    "401": {
      "statusCode": 302,
      "redirect": "/.auth/login/aad"
    }
  },
  "auth": {
    "identityProviders": {
      "azureActiveDirectory": {
        "registration": {
          "openIdIssuer": "https://login.microsoftonline.com/<TENANT_ID>/v2.0",
          "clientIdSettingName": "AZURE_CLIENT_ID",
          "clientSecretSettingName": "AZURE_CLIENT_SECRET"
        }
      }
    }
  }
}
```

## Prerequisites

When available, check this skill's declared setup contract first:

```bash
mise-en-place setup azure-static-site --capability deploy
```

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

By default, the user also needs permission to create an Entra app registration or an admin-provided client ID and secret that can be stored as Static Web App app settings.

## Manual Commands

Use these only if the helper does not fit the request. These low-level commands do not configure company-only auth by themselves.

```bash
RG="$AZURE_STATIC_SITE_RESOURCE_GROUP"
test -n "$RG"
az group show -n "$RG" -o none
az staticwebapp create -n "$SWA" -g "$RG" -l "$LOC" --sku "$SKU"

TOKEN=$(az staticwebapp secrets list \
  -n "$SWA" \
  -g "$RG" \
  --query "properties.apiKey" \
  -o tsv)

swa deploy "$SITE_DIR" --deployment-token "$TOKEN" --env production

HOST=$(az staticwebapp show \
  -n "$SWA" \
  -g "$RG" \
  --query "defaultHostname" \
  -o tsv)

echo "https://$HOST"
```

## Official Docs

- Azure Static Web Apps overview: https://learn.microsoft.com/azure/static-web-apps/overview
- Static Web Apps CLI: https://learn.microsoft.com/azure/static-web-apps/static-web-apps-cli
- Static Web Apps auth: https://learn.microsoft.com/azure/static-web-apps/authentication-authorization
- Custom auth provider: https://learn.microsoft.com/azure/static-web-apps/authentication-custom
- Configuration file: https://learn.microsoft.com/azure/static-web-apps/configuration
