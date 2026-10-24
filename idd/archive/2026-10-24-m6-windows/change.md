# m6-windows

## Intent and acceptance

M6: netmon-tray compila y funciona en Windows con las mismas funciones: motor, sondas sin privilegios, historial con bloqueo, contrato de control v1, icono de bandeja, arranque con la sesión y binarios de Windows en el release

Acceptance: GOOS=windows go build y go vet pasan; en Windows el motor sondea sin privilegios, guarda el historial, muestra el icono de bandeja, atiende netmon-tray ctl con el contrato v1, rechaza un segundo motor y service install/remove lo arranca con la sesión; el release publica binarios de Windows

## Plan

1. `internal/adapter/filelock`: bloqueo exclusivo sin espera (`flock` en Unix, `LockFileEx` en Windows) que comparten el historial (`history.db.lock`) y el socket (`control.lock`); `storage/lock.go` desaparece.
2. `internal/adapter/control`: lo dependiente del sistema pasa a `server_unix.go` y `server_windows.go` (directorio privado, socket, dueño y par, errores de bind y de conexión). En Windows, `%LOCALAPPDATA%\netmon-tray` con DACL protegida solo del SID del usuario y comprobación del SID del par con `SIO_AF_UNIX_GETPEERPID`. El cliente da «no está en marcha» también cuando falta el socket.
3. `internal/adapter/probe`: ping Unix a `ping_unix.go`; en Windows, `IcmpSendEcho` de `iphlpapi` y la ruta por defecto de `GetIpForwardTable2` (métrica de ruta más métrica de interfaz) con el estado de `GetIfEntry2Ex`; la elección de ruta (`pickRoute`) es portable y se prueba en Linux.
4. `internal/adapter/logging`: `OpenFile` con rotación a `.old` al arrancar si pasa de 1 MiB; si no puede mover el log, sigue añadiendo.
5. `internal/adapter/runkey`: `service install/remove` de Windows con la clave `Run` de `HKCU`, lógica portable probada con un registro falso y acceso real en `registry_windows.go`.
6. `cmd/netmon-tray`: uso, rutas, log, `plugin` y `service` por sistema (`platform_unix.go`, `platform_windows.go`).
7. CI con job `windows-latest`, release con pruebas en Windows y zips `windows-amd64`/`windows-arm64` con `netmon-tray.exe` y `netmon-trayw.exe`; README, contrato de control, línea base técnica y roadmap; check local `windows-vet`.

## Decisions

- Canal de control en Windows: el mismo socket AF_UNIX y contrato v1 en `%LOCALAPPDATA%\netmon-tray\control.sock`, con ACL solo del usuario (respuesta del usuario: «todas las recomendadas»). Además, el servidor rechaza la conexión cuyo proceso es de otro SID (`SIO_AF_UNIX_GETPEERPID`), que cumple la fitness del ADR 0005 igual que en Linux (respuesta del usuario: «si, mejor»).
- Datos y registro en `%LOCALAPPDATA%\netmon-tray`; el log va a `netmon-tray.log`, que se recorta al arrancar renombrándolo a `.old` si pasa de 1 MiB (respuesta del usuario; el renombrado conserva el último tramo).
- Dos binarios del mismo código: `netmon-tray.exe` de consola para `ctl`, `service` y `version`, y `netmon-trayw.exe` (`-H windowsgui`) que arranca la clave `Run`. Un único binario gráfico perdía la salida y los códigos de `ctl` porque `cmd.exe` y PowerShell no esperan a un programa gráfico (respuesta del usuario: «si, mejor»).
- `service install` escribe el valor `netmon-tray` en `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` y arranca el motor si no hay uno; con uno en marcha solo registra y sale con 0. `remove` borra el valor y no para el motor, porque el contrato v1 no tiene orden de parada. Un servicio de Windows no vale: corre en la sesión 0, sin bandeja.
- `netmon-tray plugin` en Windows sale con 2 y no aparece en el uso (respuesta del usuario).
- Verificación: check local `windows-vet` (`env GOOS=windows go vet ./...`), job `windows-latest` en CI y en el release, y prueba manual en un Windows real antes del tag (respuesta del usuario). El código de Windows no se ejecuta en este equipo (sin wine), por petición del usuario.
- `filelock` es un paquete nuevo porque historial y socket necesitan el mismo bloqueo en dos sistemas; las pruebas de omarchy y systemd se excluyen en Windows porque esos adaptadores solo existen en Linux.
- Release con permisos mínimos (sugerencia de la revisión de confianza): el workflow tiene `contents: read`; `windows-test` y `build` prueban y compilan con lectura y suben los binarios como artefacto, y solo `publish`, con `contents: write`, los descarga y crea el release.
- El contrato de control decía que un segundo motor sale con 1; desde M5 sale con 4 y se corrige al describir el canal de Windows.

## Evidence

<!-- ospec:evidence:start -->
- ev-1: migration-test for migration-compat-and-test (2026-10-24T01:35:02.666Z)
- ev-2: check-run for checks-pass (2026-10-24T01:35:19.890Z)
- ev-3: adr-impact-declaration for adr-impact-declaration (2026-10-24T01:35:19.890Z)
- ev-4: frozen-review for trust-review (2026-10-24T01:40:00.345Z, subagent-stop)
- ev-5: check-run for checks-pass (2026-10-24T01:40:23.326Z)
- ev-6: adr-impact-declaration for adr-impact-declaration (2026-10-24T01:40:23.326Z)
- ev-7: migration-test for migration-compat-and-test (2026-10-24T01:40:30.603Z)
- ev-8: frozen-review for trust-review (2026-10-24T01:41:02.194Z, relayed)
- ev-9: living-doc-current for living-doc (2026-10-24T01:41:25.608Z)
<!-- ospec:evidence:end -->
