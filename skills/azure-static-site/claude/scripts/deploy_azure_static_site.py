#!/usr/bin/env python3
"""Deploy or update a local static folder to Azure Static Web Apps."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any


RESOURCE_GROUP_ENV = "AZURE_STATIC_SITE_RESOURCE_GROUP"


def fail(message: str) -> None:
    print(f"error: {message}", file=sys.stderr)
    sys.exit(1)


def run(cmd: list[str], *, capture: bool = False, secret: bool = False) -> str:
    display = " ".join(cmd if not secret else [cmd[0], *cmd[1:3], "..."])
    if not capture:
        print(f"+ {display}", file=sys.stderr)
    proc = subprocess.run(
        cmd,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
    )
    if proc.returncode != 0:
        if capture and proc.stderr:
            print(proc.stderr.rstrip(), file=sys.stderr)
        fail(f"command failed: {display}")
    return proc.stdout.strip() if capture and proc.stdout else ""


def require_tool(name: str, install_hint: str | None = None) -> None:
    if shutil.which(name):
        return
    hint = f" Install it with: {install_hint}" if install_hint else ""
    fail(f"missing required command `{name}`.{hint}")


def load_ref(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text())
    except json.JSONDecodeError as exc:
        fail(f"invalid JSON in {path}: {exc}")


def save_ref(path: Path, data: dict[str, Any]) -> None:
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")


def sanitize_name(value: str) -> str:
    value = value.lower()
    value = re.sub(r"[^a-z0-9-]+", "-", value)
    value = re.sub(r"-+", "-", value).strip("-")
    if not value:
        value = "site"
    return value[:48].strip("-") or "site"


def az_account_value(query: str) -> str:
    return run(["az", "account", "show", "--query", query, "-o", "tsv"], capture=True)


def static_app_exists(name: str, resource_group: str) -> bool:
    proc = subprocess.run(
        ["az", "staticwebapp", "show", "-n", name, "-g", resource_group, "-o", "none"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    return proc.returncode == 0


def group_exists(resource_group: str) -> bool:
    proc = subprocess.run(
        ["az", "group", "exists", "-n", resource_group],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if proc.returncode != 0:
        combined = "\n".join(part for part in [proc.stdout.strip(), proc.stderr.strip()] if part)
        if "Forbidden" in combined:
            fail(
                "Azure returned Forbidden while checking the resource group. "
                f"Set {RESOURCE_GROUP_ENV} to the approved existing resource group, or ask an Azure admin "
                "to grant access to that configured resource group. Refusing to fall back to another "
                "visible resource group."
            )
        fail(f"could not check resource group {resource_group}: {combined or 'az group exists failed'}")
    return proc.stdout.strip().lower() == "true"


def resource_group_location(resource_group: str) -> str:
    return run(
        ["az", "group", "show", "-n", resource_group, "--query", "location", "-o", "tsv"],
        capture=True,
    )


def configured_resource_group(ref: dict[str, Any], resource_group_arg: str | None) -> str:
    if resource_group_arg:
        fail(f"--resource-group is disabled; set {RESOURCE_GROUP_ENV} to the approved resource group instead")

    env_value = os.environ.get(RESOURCE_GROUP_ENV, "").strip()
    if not env_value:
        fail(f"{RESOURCE_GROUP_ENV} is required and must name the approved Azure resource group")

    ref_value = str(ref.get("resourceGroup") or "").strip()
    if ref_value and ref_value != env_value:
        fail(
            f"{RESOURCE_GROUP_ENV}={env_value} conflicts with ref file resourceGroup={ref_value}. "
            "Refusing to deploy across resource groups."
        )

    return env_value


def write_auth_config(site_dir: Path, tenant_id: str) -> None:
    config_path = site_dir / "staticwebapp.config.json"
    if config_path.exists():
        print(f"info: leaving existing {config_path}; verify it includes custom Entra auth", file=sys.stderr)
        return
    config = {
        "routes": [{"route": "/*", "allowedRoles": ["authenticated"]}],
        "responseOverrides": {
            "401": {"statusCode": 302, "redirect": "/.auth/login/aad"}
        },
        "auth": {
            "identityProviders": {
                "azureActiveDirectory": {
                    "registration": {
                        "openIdIssuer": f"https://login.microsoftonline.com/{tenant_id}/v2.0",
                        "clientIdSettingName": "AZURE_CLIENT_ID",
                        "clientSecretSettingName": "AZURE_CLIENT_SECRET",
                    }
                }
            }
        },
    }
    save_ref(config_path, config)
    print(f"wrote {config_path}", file=sys.stderr)


def create_auth_app(name: str, resource_group: str, host: str) -> str:
    app_id = run(
        [
            "az",
            "ad",
            "app",
            "create",
            "--display-name",
            f"{name}-auth",
            "--sign-in-audience",
            "AzureADMyOrg",
            "--web-redirect-uris",
            f"https://{host}/.auth/login/aad/callback",
            "--query",
            "appId",
            "-o",
            "tsv",
        ],
        capture=True,
    )
    if not app_id:
        fail("az ad app create did not return an appId")

    secret = run(
        [
            "az",
            "ad",
            "app",
            "credential",
            "reset",
            "--id",
            app_id,
            "--append",
            "--display-name",
            "swa-auth-secret",
            "--years",
            "1",
            "--query",
            "password",
            "-o",
            "tsv",
        ],
        capture=True,
        secret=True,
    )
    if not secret:
        fail("az ad app credential reset did not return a secret")

    run(
        [
            "az",
            "staticwebapp",
            "appsettings",
            "set",
            "-n",
            name,
            "-g",
            resource_group,
            "--setting-names",
            f"AZURE_CLIENT_ID={app_id}",
            f"AZURE_CLIENT_SECRET={secret}",
        ],
        secret=True,
    )
    return app_id


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Deploy or update a static folder to Azure Static Web Apps."
    )
    parser.add_argument("--dir", default=".", help="Static site folder containing index.html")
    parser.add_argument("--ref-file", help="Path to .azure-static-site.json")
    parser.add_argument("--name", help="Azure Static Web App name")
    parser.add_argument("--resource-group", help=f"Disabled. Use {RESOURCE_GROUP_ENV}.")
    parser.add_argument("--location", help="Azure region for new apps")
    parser.add_argument("--subscription", help="Azure subscription id or name")
    parser.add_argument("--sku", choices=["Free", "Standard"], help="SKU for new apps")
    parser.add_argument(
        "--company-auth",
        action="store_true",
        help="Configure single-tenant Microsoft Entra auth for company-only access",
    )
    parser.add_argument("--tenant-id", help="Microsoft Entra tenant id")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    require_tool("az")
    require_tool("swa", "npm i -g @azure/static-web-apps-cli")

    site_dir = Path(args.dir).expanduser().resolve()
    if not site_dir.is_dir():
        fail(f"site dir does not exist: {site_dir}")
    if not (site_dir / "index.html").is_file():
        fail(f"site dir must contain index.html: {site_dir}")

    ref_file = Path(args.ref_file).expanduser().resolve() if args.ref_file else site_dir / ".azure-static-site.json"
    ref = load_ref(ref_file)

    if args.subscription:
        run(["az", "account", "set", "--subscription", args.subscription])

    subscription_id = az_account_value("id")
    tenant_id = args.tenant_id or ref.get("tenantId") or az_account_value("tenantId")

    resource_group = configured_resource_group(ref, args.resource_group)
    default_name = f"swa-{sanitize_name(site_dir.name)}"
    name = args.name or ref.get("name") or default_name
    if not group_exists(resource_group):
        fail(
            f"resource group {resource_group} does not exist or is not visible to this account. "
            f"Set {RESOURCE_GROUP_ENV} to an approved existing resource group."
        )

    location = args.location or ref.get("location") or resource_group_location(resource_group)
    sku = args.sku or ref.get("sku") or ("Standard" if args.company_auth else "Free")
    if args.company_auth and sku != "Standard":
        if ref.get("name") or args.name:
            fail("company auth requires a Standard Static Web Apps SKU; upgrade the existing app or create a new Standard app")
        print("info: company auth requested; using Standard SKU for new app", file=sys.stderr)
        sku = "Standard"

    exists = static_app_exists(name, resource_group)
    if not exists:
        run(
            [
                "az",
                "staticwebapp",
                "create",
                "-n",
                name,
                "-g",
                resource_group,
                "-l",
                location,
                "--sku",
                sku,
                "-o",
                "none",
            ]
        )
    else:
        print(f"info: updating existing Static Web App {resource_group}/{name}", file=sys.stderr)

    host = run(
        [
            "az",
            "staticwebapp",
            "show",
            "-n",
            name,
            "-g",
            resource_group,
            "--query",
            "defaultHostname",
            "-o",
            "tsv",
        ],
        capture=True,
    )
    if not host:
        fail("could not resolve Static Web App defaultHostname")

    auth_app_id = ref.get("authAppId")
    if args.company_auth:
        write_auth_config(site_dir, tenant_id)
        if not auth_app_id:
            auth_app_id = create_auth_app(name, resource_group, host)
        else:
            print(f"info: using existing auth app id from ref file: {auth_app_id}", file=sys.stderr)

    token = run(
        [
            "az",
            "staticwebapp",
            "secrets",
            "list",
            "-n",
            name,
            "-g",
            resource_group,
            "--query",
            "properties.apiKey",
            "-o",
            "tsv",
        ],
        capture=True,
        secret=True,
    )
    if not token:
        fail("could not fetch Static Web Apps deployment token")

    run(["swa", "deploy", str(site_dir), "--deployment-token", token, "--env", "production"], secret=True)

    url = f"https://{host}"
    ref.update(
        {
            "schema": 1,
            "provider": "azure-static-web-apps",
            "resourceGroup": resource_group,
            "name": name,
            "location": location,
            "sku": sku,
            "subscriptionId": subscription_id,
            "tenantId": tenant_id,
            "defaultHostname": host,
            "url": url,
            "lastDeployedAt": dt.datetime.now(dt.timezone.utc).isoformat(),
            "deploymentTokenStorage": "not stored; fetched with az staticwebapp secrets list",
        }
    )
    if auth_app_id:
        ref["authAppId"] = auth_app_id
    save_ref(ref_file, ref)

    print(json.dumps({"url": url, "refFile": str(ref_file), "resourceGroup": resource_group, "name": name}, indent=2))


if __name__ == "__main__":
    main()
