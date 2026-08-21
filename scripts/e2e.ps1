# Windows wrapper for scripts/e2e.sh.
#
# This machine has no native bash, so the suite runs inside a docker:cli
# container (alpine + docker CLI) with bash/curl/node added:
#   - API_URL points at the host through host.docker.internal;
#   - the docker socket is mounted so the tenant-isolation seeding
#     (docker exec into postgres) works;
#   - the webhook receiver port is published so the host API can call the
#     in-container node listener back.
#
# Usage: powershell -File scripts\e2e.ps1  (infra + API + worker must be up)

param(
    [string]$ApiPort = "8081",
    [string]$AdminPassword = $(if ($env:BOOTSTRAP_ADMIN_PASSWORD) { $env:BOOTSTRAP_ADMIN_PASSWORD } else { "change-me-please" }),
    [string]$WebhookPort = "39991",
    [string]$StalwartHttpPort = "8180",
    [string]$StalwartAdminPassword = $(if ($env:STALWART_ADMIN_PASSWORD) { $env:STALWART_ADMIN_PASSWORD } else { "stalwart_dev_admin" }),
    [string]$StalwartMasterPassword = $(if ($env:STALWART_MASTER_PASSWORD) { $env:STALWART_MASTER_PASSWORD } else { "stalwart_dev_master" })
)

$repo = Resolve-Path (Join-Path $PSScriptRoot "..")
docker run --rm `
    -v "${repo}:/w" -w /w `
    -v "//var/run/docker.sock:/var/run/docker.sock" `
    -p "${WebhookPort}:${WebhookPort}" `
    -e "API_URL=http://host.docker.internal:${ApiPort}" `
    -e "BOOTSTRAP_ADMIN_PASSWORD=${AdminPassword}" `
    -e "WEBHOOK_PORT=${WebhookPort}" `
    -e "STALWART_HTTP=http://host.docker.internal:${StalwartHttpPort}" `
    -e "STALWART_ADMIN_PASSWORD=${StalwartAdminPassword}" `
    -e "STALWART_MASTER_PASSWORD=${StalwartMasterPassword}" `
    docker:cli sh -c "apk add -q bash curl nodejs && tr -d '\r' < scripts/e2e.sh > /tmp/e2e.sh && cd /w && bash /tmp/e2e.sh"
exit $LASTEXITCODE
