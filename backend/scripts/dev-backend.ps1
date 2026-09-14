<#
.SYNOPSIS
  Local-dev launcher for the Together backend.

.DESCRIPTION
  Builds the API binary, loads EVERY valid KEY=VALUE entry from backend/.env
  into this process's environment (not just DATABASE_URL), then starts the
  built exe detached with the existing log behavior.

  Restart model (PID-file based, no network/port scanning): the launcher records
  the backend's PID in a PID file under backend\.dev-bin. On the next run it stops
  ONLY that recorded PID, and only after confirming that PID still points at our
  own binary, then waits for it to exit before starting a fresh instance. It does
  NOT make network calls or enumerate ports/processes (that behavior tripped AV
  heuristics). Readiness (/health, /ready) and single-listener checks are done by
  the caller, not by this script.

  Secret safety: values from .env are set into the environment but never printed.
  Only the KEY NAMES that were loaded are echoed, plus a boolean "present" flag
  for the translation provider/key.

.NOTES
  - Preserves port 8080 (from .env / backend default).
  - Preserves log files at %TEMP%\together-api.log (stdout) and .err (stderr).
  - Ignores blank lines and lines beginning with '#'.
  - Never commits or dumps secret values.
  - ASCII only: Windows PowerShell 5.1 misreads non-ASCII in a BOM-less file.
#>

[CmdletBinding()]
param(
    # Skip the build step and reuse an already-built exe.
    [switch] $NoBuild
)

$ErrorActionPreference = 'Stop'

# Resolve backend root as the parent of this script's directory (backend\scripts).
$backendRoot = Split-Path -Parent $PSScriptRoot
$envPath = Join-Path $backendRoot '.env'
# Build/run from a stable, git-ignored project-local dir instead of %TEMP%: a
# stable path is less "dropper-like" to AV heuristics than a temp-dir binary.
$devBinDir = Join-Path $backendRoot '.dev-bin'
$exePath = Join-Path $devBinDir 'together-api.exe'
$pidFile = Join-Path $devBinDir 'together-api.pid'
$logOut = Join-Path $env:TEMP 'together-api.log'
$logErr = "$logOut.err"

if (-not (Test-Path $envPath)) {
    throw "backend/.env not found at $envPath. Create it before starting the backend."
}

# --- Build (unless skipped) -------------------------------------------------
if (-not $NoBuild) {
    Write-Host 'Building backend...'
    if (-not (Test-Path $devBinDir)) {
        New-Item -ItemType Directory -Path $devBinDir | Out-Null
    }
    Push-Location $backendRoot
    try {
        & go build -o $exePath ./cmd/api
        if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
}

if (-not (Test-Path $exePath)) {
    throw "backend exe not found at $exePath. Run without -NoBuild first."
}

# --- Load ALL KEY=VALUE entries from .env -----------------------------------
# Split on the FIRST '=' only, so values may themselves contain '='. Surrounding
# single/double quotes are stripped. Blank lines and '#' comments are ignored.
$loadedKeys = New-Object System.Collections.Generic.List[string]
foreach ($raw in Get-Content -LiteralPath $envPath) {
    $line = $raw.Trim()
    if ($line -eq '' -or $line.StartsWith('#')) { continue }
    $eq = $line.IndexOf('=')
    if ($eq -lt 1) { continue }  # no key, or '=' at start -> skip
    $key = $line.Substring(0, $eq).Trim()
    $val = $line.Substring($eq + 1).Trim()
    if ($val.Length -ge 2) {
        $first = $val[0]; $last = $val[$val.Length - 1]
        if (($first -eq '"' -and $last -eq '"') -or ($first -eq "'" -and $last -eq "'")) {
            $val = $val.Substring(1, $val.Length - 2)
        }
    }
    if ($key -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') { continue }  # skip malformed keys
    Set-Item -Path ("Env:" + $key) -Value $val
    $loadedKeys.Add($key)
}

$ourExeFull = [System.IO.Path]::GetFullPath($exePath)

# --- Stop ONLY our previously-recorded backend PID, then wait for exit -------
if (Test-Path $pidFile) {
    $recorded = Get-Content -LiteralPath $pidFile -ErrorAction SilentlyContinue | Select-Object -First 1
    $oldPid = 0
    [void][int]::TryParse((($recorded -as [string])).Trim(), [ref]$oldPid)
    if ($oldPid -gt 0) {
        $old = $null
        try { $old = Get-Process -Id $oldPid -ErrorAction Stop } catch { $old = $null }
        if ($old) {
            # Only stop it if that PID is still OUR binary (guards against a PID
            # recycled by an unrelated process).
            $isOurs = $false
            try {
                if ($old.Path -and ([System.IO.Path]::GetFullPath($old.Path) -ieq $ourExeFull)) {
                    $isOurs = $true
                }
            } catch { $isOurs = $false }
            if ($isOurs) {
                try { Stop-Process -Id $oldPid -Force -ErrorAction Stop } catch {}
                try { Wait-Process -Id $oldPid -Timeout 10 -ErrorAction SilentlyContinue } catch {}
            }
        }
    }
    Remove-Item -LiteralPath $pidFile -ErrorAction SilentlyContinue
}

# --- Start detached, then record its PID ------------------------------------
$proc = Start-Process -FilePath $exePath -WorkingDirectory $backendRoot `
    -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput $logOut -RedirectStandardError $logErr
Set-Content -LiteralPath $pidFile -Value ([string]$proc.Id) -Encoding ascii

# --- Report (names + presence only, never values; no network calls) ---------
Write-Host ("Backend PID: " + $proc.Id)
Write-Host ("Loaded env keys: " + ($loadedKeys -join ', '))
$provider = $env:TRANSLATION_PROVIDER
if ([string]::IsNullOrEmpty($provider)) { $provider = '(unset -> stub default)' }
Write-Host ("Translation provider: " + $provider)
Write-Host ("Translation API key present: " + [bool]$env:TRANSLATION_API_KEY)
Write-Host ("PID file: " + $pidFile)
Write-Host ("Logs: " + $logOut + " (stdout), " + $logErr + " (stderr)")
Write-Host ("Started. Verify readiness with: GET http://localhost:8080/health and /ready")
