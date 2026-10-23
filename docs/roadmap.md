# Hoja de ruta — netmon-tray

## Dirección

Un motor local en Go que detecta y clasifica cortes de red, guarda 2 meses de historial exportable y lo muestra en un panel propio de Omarchy, como alexa-tray. Windows llega después con los mismos casos de uso y otra presentación.

## Primer slice: walking skeleton

Binario único con dominio y adaptadores separados (ADR 0002), sondas por capas sin privilegios (ADR 0004), clasificación de un corte, historial local versionado (ADR 0003) e icono SNI con el estado real. Registro estructurado. Sin panel QML ni exportación todavía.

## Hitos

| Hito | Alcance |
| --- | --- |
| M1 — Walking skeleton | El primer slice |
| M2 — Exportación y retención | Exportar a CSV/JSON, rotación a 60 días y migración de esquema |
| M3 — Contrato de control | Socket local versionado y `ctl` (ADR 0005) |
| M4 — Panel de Omarchy | Plugin QML con estado y último corte, instalable desde el binario |
| M5 — Servicio e instalación | Unidad `systemd --user`, README, `go install` y release en GitHub |
| M6 — Windows | Adaptadores de bandeja y servicio para Windows |

## Decisiones diferidas

| Decisión | Responsable | Último momento responsable |
| --- | --- | --- |
| Biblioteca de almacén del historial | Autor | Antes de escribir el almacén en M1 |
| Biblioteca de bandeja SNI | Autor | Antes de escribir el adaptador de icono en M1 |
| Objetivos, intervalos y umbrales por defecto de las sondas | Autor | Al medir consumo y detección en M1 |
| Umbrales numéricos de consumo y de detección | Autor | Al cerrar M1, tras medir |
| Canal de control y presentación en Windows | Autor | Antes de empezar M6 |
