# m2-exportacion-retencion

## Intent and acceptance

Exportar el historial a CSV/JSON desde la bandeja, purgar cortes de más de 60 días y migrar el esquema a la versión 2

Acceptance: El ítem Exportar… del menú escribe netmon-historial-<fecha>.csv y .json en exports/ con todos los cortes; el motor descarta los cortes cerrados hace más de 60 días al arrancar y cada 24 h; un archivo v1 abre y migra a v2 sin perder cortes

## Plan

1. Almacén: migración v2 (índice sobre `ended_at`), `List` (más antiguo primero) y `Purge` (solo cortes cerrados).
2. Adaptador `export`: `CSV`, `JSON` y `Exporter.Write` (ambos archivos o ninguno, permisos 0600/0700).
3. Motor: opciones `WithRetention` y `WithExporter`, purga al arrancar y cada 24 h, `Engine.Export`.
4. Bandeja y `main`: ítem «Exportar historial», línea con el resultado y cableado; prueba de integración de reinicio, rotación y exportación.

## Decisions

- Confirmadas por el usuario («todas las recomendadas»): disparo desde la bandeja; destino `~/.local/share/netmon-tray/exports/netmon-historial-<fecha>.{csv,json}`; columnas `inicio,fin,duracion_s,causa,abierto`; RFC3339 en hora local con desfase; 60 días desde el fin del corte, los abiertos no se borran; purga al abrir y cada 24 h; migración v2 con índice; `ping()` y el icono de Omarchy quedan fuera.
- JSON: un array de objetos con las mismas claves que el CSV; un corte abierto lleva `"fin": null` (el equivalente de «fin vacío» del CSV).
- `duracion_s` son segundos enteros redondeados; un corte abierto cuenta hasta `LastSeen`.
- `causa` conserva el identificador estable del dominio (`link`, `router`, `line`), no el texto en español de la bandeja.
- Una exportación fallida borra el archivo ya escrito; dos exportaciones en el mismo segundo sobrescriben el mismo nombre.
- El motor no tiene reloj propio: `now` se inyecta para fijar el corte de la retención en las pruebas.

## Evidence

<!-- ospec:evidence:start -->
- ev-1: migration-test for migration-compat-and-test (2026-10-23T19:48:51.108Z)
- ev-2: check-run for checks-pass (2026-10-23T19:50:43.795Z)
- ev-3: adr-impact-declaration for adr-impact-declaration (2026-10-23T19:50:43.795Z)
- ev-4: migration-test for migration-compat-and-test (2026-10-23T19:50:51.237Z)
- ev-5: check-run for checks-pass (2026-10-23T19:50:51.672Z)
- ev-6: adr-impact-declaration for adr-impact-declaration (2026-10-23T19:50:51.672Z)
- ev-7: living-doc-current for living-doc (2026-10-23T19:50:59.263Z)
<!-- ospec:evidence:end -->
