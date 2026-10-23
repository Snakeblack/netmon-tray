# m1-walking-skeleton

## Intent and acceptance

Motor Go de proceso único: sondas por capas sin privilegios, clasificación de cortes, historial local versionado e icono SNI con el estado real, con logs estructurados

Acceptance: Al ejecutar el binario en Omarchy el icono SNI refleja si hay conexión y en qué capa falla; un corte real o simulado queda guardado con inicio, duración y clasificación y sobrevive a reiniciar el proceso

## Plan

1. `internal/domain`: capas, `Classify` (capa más cercana que falla) y `Tracker` (umbral de fallos consecutivos, fusión de cortes cercanos).
2. `internal/adapter/storage`: SQLite puro Go con `user_version`, migraciones y permisos 0600.
3. `internal/adapter/probe`: ICMP por ping sockets sin privilegios, ruta por defecto desde `/proc/net/route` y DNS con etiqueta aleatoria.
4. `internal/app`: `Engine` con puertos `Prober`, `Store` y `Sink`; reintenta guardados fallidos.
5. `internal/adapter/tray` y `cmd/netmon-tray`: icono SNI con 4 estados y menú, cableado y registro JSON.

## Decisions

- Valores confirmados por el usuario: sondeo cada 2 s, corte tras 3 fallos consecutivos, fusión de cortes separados por menos de 10 s, almacén `modernc.org/sqlite`, bandeja `fyne.io/systray`, icono de 4 estados por color y tooltip en español.
- Suposiciones: línea y operador forman una sola causa (`line`) porque las sondas sin privilegios no los distinguen; sin archivo de configuración, solo constantes en `main.go`; un fallo del almacén o de la barra no detiene el sondeo.
- Un corte solo se registra tras 3 fallos, por lo que el mínimo registrable es de unos 6 s; su inicio se fija en el primer fallo.
- Al abrir el almacén, un corte que quedó abierto por la muerte del proceso se cierra en su último instante visto.
- El DNS se mide cada 10 s con una etiqueta aleatoria bajo `example.com` para saltar la caché del resolvedor; NXDOMAIN cuenta como respuesta.
- Si el sistema no permite ICMP sin privilegios (`net.ipv4.ping_group_range`), el binario se niega a arrancar en vez de inventar cortes.
- El archivo del historial se crea con 0600 antes de abrirlo y el directorio se fuerza a 0700: `-wal` y `-shm` heredan el modo del archivo principal (hallazgo de la revisión de confianza).
- Sugerencia de la revisión sin aplicar: `ping()` acepta cualquier echo reply sin comprobar el origen; queda como trabajo posterior.
- Pendiente para M2: rotación a 60 días y exportación (ADR 0003).

## Evidence

<!-- ospec:evidence:start -->
- ev-1: migration-test for migration-compat-and-test (2026-10-23T19:19:52.441Z)
- ev-2: check-run for checks-pass (2026-10-23T19:20:24.610Z)
- ev-3: adr-impact-declaration for adr-impact-declaration (2026-10-23T19:20:24.610Z)
- ev-4: frozen-review for trust-review (2026-10-23T19:21:41.767Z, relayed)
- ev-5: check-run for checks-pass (2026-10-23T19:21:58.149Z)
- ev-6: adr-impact-declaration for adr-impact-declaration (2026-10-23T19:21:58.149Z)
- ev-7: migration-test for migration-compat-and-test (2026-10-23T19:22:03.701Z)
- ev-8: frozen-review for trust-review (2026-10-23T19:23:02.969Z, relayed)
- ev-9: check-run for checks-pass (2026-10-23T19:23:03.268Z)
- ev-10: adr-impact-declaration for adr-impact-declaration (2026-10-23T19:23:03.268Z)
- ev-11: living-doc-current for living-doc (2026-10-23T19:23:09.064Z)
<!-- ospec:evidence:end -->
