# m4-panel-omarchy

## Intent and acceptance

Panel de Omarchy: plugin QML embebido en el binario que muestra el estado de la red y el último corte consumiendo netmon-tray ctl watch, instalable con netmon-tray plugin install

Acceptance: netmon-tray plugin install copia el plugin a ~/.config/omarchy/plugins/<id> y pone el widget en la barra; el botón refleja el estado (conectado, degradado, caído, sin motor) y el panel muestra estado, causa y último corte al día vía ctl watch, sin leer el historial

## Plan

1. `omarchy-plugin/`: `manifest.json`, `Panel.qml` (botón de barra y panel) y `Model.js` (lógica pura con pruebas `node --test`), embebidos en el binario con `embed.go`.
2. `internal/adapter/omarchy`: `netmon-tray plugin install` copia el plugin, pide a omarchy-shell que lo descubra, pone el widget en la barra y oculta una vez el icono SNI.
3. `main`: subcomando `plugin`; `tray.Title` da el id SNI que se oculta.
4. `idd/config.yaml`: check `plugin: node --test omarchy-plugin/`, aprobado por el usuario.

## Decisions

- Confirmadas por el usuario («todas las recomendadas»): id `snakeblack.netmon-tray`; la instalación oculta una vez el icono SNI y lo apunta con `trayIconHidden`; hora `02/01 15:04:05` y duración legible (`1 min 5 s`) con contador en vivo del corte abierto; botón **Exportar historial** con la carpeta y **Abrir carpeta**; sin motor, botón atenuado, **Iniciar** (`systemctl --user start netmon-tray.service` o binario suelto) y reintento de `watch` cada 5 s; un glifo por estado más color de aviso, tooltip con el texto del estado; clic central sin acción.
- El instalador y sus pruebas se copian de alexa-tray y se adaptan: dos repositorios independientes sin un módulo compartido que justifique la dependencia.
- El panel solo usa `ctl watch` y `ctl export --json`: el contrato v1 no cambia.
- Un corte abierto se cuenta desde `start` con un temporizador local de 1 s, que solo corre con el panel abierto y el corte en curso, porque el motor no emite eventos por la duración.
- Un estado degradado con causa la nombra en el panel («Conexión degradada: falla el router»), algo que `ctl status` no hace.
- Quickshell no emite `exited` si el comando no arranca: el reintento se programa en `onRunningChanged`, y `onStarted` distingue un comando que no existe de un `ctl` que termina. Al cambiar el comando, el reinicio espera con `Qt.callLater` a que se actualice el binding del proceso; si no, arranca con el comando anterior. alexa-tray tiene el mismo fallo (pendiente de corregir allí).
- El README con la instalación del panel llega con M5 (no hay README todavía); `netmon-tray plugin help` muestra el uso.
- La señal `persistent-data` sale de las escrituras del instalador (copia del plugin y `shell.json`); el historial no cambia. Se satisfizo con las pruebas de actualización del instalador.

## Evidence

<!-- ospec:evidence:start -->
- ev-1: check-run for checks-pass (2026-10-23T23:52:16.274Z)
- ev-2: adr-impact-declaration for adr-impact-declaration (2026-10-23T23:52:16.274Z)
- ev-3: migration-test for migration-compat-and-test (2026-10-23T23:52:26.030Z)
- ev-4: check-run for checks-pass (2026-10-23T23:52:49.188Z)
- ev-5: adr-impact-declaration for adr-impact-declaration (2026-10-23T23:52:49.188Z)
- ev-6: living-doc-current for living-doc (2026-10-23T23:52:57.165Z)
<!-- ospec:evidence:end -->
