# m5-servicio-release

## Intent and acceptance

M5: netmon-tray service install/remove instala el motor como unidad systemd --user, netmon-tray version muestra la versión del tag, README de primer uso y release de binarios en GitHub

Acceptance: netmon-tray service install deja netmon-tray.service habilitada y en marcha con el binario instalado; otro motor en marcha sale con un código que la unidad no reintenta; netmon-tray version imprime la versión; el README explica instalación, actualización y desinstalación; un tag v* publica binarios con pruebas previas

## Plan

1. `internal/adapter/systemd`: instalador de la unidad copiado de alexa-tray y adaptado (sin `Environment=PATH`, porque el motor no lanza binarios externos), con sus pruebas.
2. `cmd/netmon-tray`: subcomandos `service` y `version`/`--version`, uso y salida 2 ante un argumento desconocido, salida 4 (`systemd.ExitAlreadyRunning`) cuando otro motor tiene el historial o el socket, y la versión en el registro de arranque.
3. Publicación: `LICENSE` MIT, `README.md`, `.github/workflows/ci.yml` (vet, test y node en push y PR) y `.github/workflows/release.yml` (con un tag `v*`: las mismas pruebas y binarios estáticos `linux-amd64` y `linux-arm64` en `.tar.gz` con `SHA256SUMS`), y el roadmap.

## Decisions

- Unidad como alexa-tray: sigue a `graphical-session.target` con `ConditionEnvironment=WAYLAND_DISPLAY`, `Restart=on-failure` a 5 s y `RestartPreventExitStatus=4`; `service install` con un motor fuera de la unidad la habilita sin arrancarla y sale con 1 (respuesta del usuario).
- Release con GitHub Actions para amd64 y arm64 (respuesta del usuario). Los binarios se compilan en `$RUNNER_TEMP`: un fichero dentro del árbol haría que Go grabase `+dirty`, y el workflow comprueba que `version` imprime el tag.
- El repositorio público y el tag `v0.1.0` se crean tras cerrar el cambio y los commits, con una confirmación explícita del usuario y después de revisar que nada del repositorio es sensible ni hace referencia a este equipo (respuesta del usuario).
- Tras verificar en vivo `netmon-tray.service`, se deshabilita el monitor antiguo con `systemctl --user disable --now netmon.service`, dejando su unidad, su binario y sus datos (respuesta del usuario). Es una operación del equipo, fuera del repositorio.
- El instalador se copia de alexa-tray en lugar de compartir un módulo, como el del plugin en M4. Prior decision (memory): netmon-tray M4: instalador copiado de alexa-tray y contrato v1 intacto -> internal/adapter/omarchy/install.go: current.

## Evidence

<!-- ospec:evidence:start -->
- ev-1: check-run for checks-pass (2026-10-24T00:22:37.139Z)
- ev-2: adr-impact-declaration for adr-impact-declaration (2026-10-24T00:22:37.139Z)
- ev-3: check-run for checks-pass (2026-10-24T00:23:37.716Z)
- ev-4: adr-impact-declaration for adr-impact-declaration (2026-10-24T00:23:37.716Z)
- ev-5: living-doc-current for living-doc (2026-10-24T00:23:37.773Z)
<!-- ospec:evidence:end -->
