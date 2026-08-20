# Cold-start benchmark for the Next.js dev server.
#
# Wipes the Turbopack cache, starts `next dev`, and times the first (cold,
# compile-bound) request to each route plus a warm re-request. Used to compare
# dev-server configurations; run from the repo root.

param([string]$Label = "run")

$ErrorActionPreference = 'Stop'
$web = Join-Path $PSScriptRoot "..\apps\web" | Resolve-Path

if (Test-Path (Join-Path $web ".next")) { Remove-Item -Recurse -Force (Join-Path $web ".next") }

$proc = Start-Process -FilePath "cmd.exe" -ArgumentList "/c npm run dev" -WorkingDirectory $web -PassThru -WindowStyle Hidden

try {
    $ready = $false
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt 120) {
        try { $null = Invoke-WebRequest -Uri "http://localhost:3000/login" -UseBasicParsing -TimeoutSec 120; $ready = $true; break }
        catch { Start-Sleep -Milliseconds 400 }
    }
    if (-not $ready) { throw "dev server never became reachable" }

    # /login was just compiled by the readiness probe above; report it separately.
    "[$Label] boot+first /login : $([int]$sw.Elapsed.TotalMilliseconds) ms"

    foreach ($route in @("/", "/mail/inbox", "/admin")) {
        $c = [Diagnostics.Stopwatch]::StartNew()
        try { $null = Invoke-WebRequest -Uri "http://localhost:3000$route" -UseBasicParsing -TimeoutSec 180 } catch {}
        $c.Stop()
        $w = [Diagnostics.Stopwatch]::StartNew()
        try { $null = Invoke-WebRequest -Uri "http://localhost:3000$route" -UseBasicParsing -TimeoutSec 180 } catch {}
        $w.Stop()
        "[$Label] {0,-14} cold {1,6} ms   warm {2,5} ms" -f $route, $c.ElapsedMilliseconds, $w.ElapsedMilliseconds
    }
}
finally {
    Get-CimInstance Win32_Process -Filter "ParentProcessId = $($proc.Id)" -ErrorAction SilentlyContinue |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue |
        ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Seconds 2
}
