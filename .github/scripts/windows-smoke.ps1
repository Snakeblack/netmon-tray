# Smoke test of the real Windows binaries, run from the repository root on a
# fresh windows-latest runner. It registers the Run value and starts the
# engine, checks what a user would, then removes both and prints the log.
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

$bin = Join-Path $env:RUNNER_TEMP 'netmon-smoke'
New-Item -ItemType Directory -Force $bin | Out-Null
$cli = Join-Path $bin 'netmon-tray.exe'
$gui = Join-Path $bin 'netmon-trayw.exe'
go build -o $cli ./cmd/netmon-tray
if ($LASTEXITCODE) { throw 'go build netmon-tray.exe' }
go build -ldflags=-H=windowsgui -o $gui ./cmd/netmon-tray
if ($LASTEXITCODE) { throw 'go build netmon-trayw.exe' }

$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$log = Join-Path $env:LOCALAPPDATA 'netmon-tray\netmon-tray.log'

function Assert-Exit([int]$want, [string]$what) {
  if ($LASTEXITCODE -ne $want) { throw "$what salió con $LASTEXITCODE, se esperaba $want" }
  Write-Host "ok: $what"
}

try {
  & $cli version; Assert-Exit 0 'version'
  & $cli ctl status; Assert-Exit 3 'ctl status sin motor'

  & $cli service install; Assert-Exit 0 'service install'
  $value = (Get-ItemProperty $runKey).'netmon-tray'
  if ($value -ne "`"$gui`"") { throw "valor Run = $value, se esperaba `"$gui`"" }
  Write-Host 'ok: valor Run'

  # The engine answers with unknown at once and changes after its first
  # probes; staying unknown means it never started probing.
  $state = $null
  $deadline = (Get-Date).AddSeconds(30)
  while ((Get-Date) -lt $deadline) {
    $out = & $cli ctl status --json 2>$null
    if ($LASTEXITCODE -eq 0) {
      $state = ($out | ConvertFrom-Json).state
      if ($state -ne 'unknown') { break }
    }
    Start-Sleep -Seconds 1
  }
  if (-not $state -or $state -eq 'unknown') { throw "el motor no ha sondeado en 30 s (estado: $state)" }
  Write-Host "ok: el motor sondea (estado: $state)"
  & $cli ctl status

  $second = Start-Process $gui -PassThru
  $null = $second.Handle # keeps the exit code readable after it exits
  if (-not $second.WaitForExit(15000)) { $second.Kill(); throw 'el segundo motor no terminó en 15 s' }
  if ($second.ExitCode -ne 4) { throw "el segundo motor salió con $($second.ExitCode), se esperaba 4" }
  Write-Host 'ok: segundo motor sale con 4'

  & $cli service remove; Assert-Exit 0 'service remove'
  if ($null -ne (Get-ItemProperty $runKey).'netmon-tray') { throw 'el valor Run sigue tras service remove' }
  Write-Host 'ok: valor Run borrado'
} finally {
  Get-Process netmon-trayw -ErrorAction SilentlyContinue | Stop-Process -Force
  Remove-ItemProperty $runKey -Name 'netmon-tray' -ErrorAction SilentlyContinue
  if (Test-Path $log) { Write-Host '--- netmon-tray.log'; Get-Content $log }
}
