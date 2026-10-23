# Alcance funcional — netmon-tray

## Actores

| Actor | Qué hace |
| --- | --- |
| Usuario de escritorio | Mira el icono y el panel, consulta el último corte y exporta el historial |
| Operador de internet | Recibe el historial exportado como prueba (no usa la herramienta) |

## Recorridos

1. **Ver el estado**: el icono y el panel muestran si hay conexión y, si no, en qué capa falla.
2. **Entender un corte**: el panel muestra el último corte con su inicio, su duración y su clasificación.
3. **Reclamar**: el usuario exporta el historial de los últimos 2 meses a CSV/JSON y lo envía al operador.

## Primer slice (walking skeleton)

Un motor en Go de proceso único que sondea por capas, detecta una caída, la clasifica y la guarda en un historial local versionado, con un icono de bandeja que muestra el estado real. Sin panel QML ni exportación todavía; ver `docs/roadmap.md`.

## Fuera de alcance

Test de velocidad, nube, telemetría, alertas externas, multiusuario, administración remota.
