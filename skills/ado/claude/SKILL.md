---
name: ado
description: "Read-only Azure DevOps lookups (work items, comments, attachments, WIQL, PRs, code search) using the Azure CLI. Use for any Azure DevOps or NPXInnovation/ECHO work item context."
---

# ado

Read only. Never create, update, or delete Azure DevOps resources.

## Auth

Azure CLI with Microsoft Entra login. No PAT is needed.

- `az login` (check with `az account show`)
- `az extension add --name azure-devops` if `az boards` is missing
- Defaults: org `https://dev.azure.com/NPXInnovation`, project `ECHO`. Pass `--org`/`--project` on each command.

## Commands

```bash
ORG=https://dev.azure.com/NPXInnovation
az boards work-item show --org $ORG --id <id> --expand all
az boards query --org $ORG --project ECHO --wiql "SELECT [System.Id], [System.Title] FROM WorkItems WHERE ..."
az repos pr list --org $ORG --project ECHO --repository <repo> --status active
az repos pr show --org $ORG --id <pr-id>
```

Anything else goes through `az rest` with the Azure DevOps resource ID:

```bash
ADO=499b84ac-1321-427f-aa17-267ca6975798
az rest --resource $ADO --url "$ORG/ECHO/_apis/wit/workItems/<id>/comments?api-version=7.1-preview.4"
az rest --resource $ADO --url "$ORG/ECHO/_apis/git/repositories/<repo>/pullRequests/<pr-id>/threads?api-version=7.1"
az rest --method post --resource $ADO \
  --url "https://almsearch.dev.azure.com/NPXInnovation/ECHO/_apis/search/codesearchresults?api-version=7.1" \
  --body '{"searchText":"<text>","$top":25}'
```

The code search call is a POST but only reads.

## Work items

Read the description, acceptance criteria, relations, and every comment. Download attachments (`AttachedFile` relations and `<img src>` URLs in HTML fields and comments) when they matter:

```bash
az rest --resource $ADO --url "<attachment-url>" --output-file /tmp/<name>
```

Read HTML fields as-is. ECHO source code and PRs live on GitHub unless the request points to Azure Repos.
