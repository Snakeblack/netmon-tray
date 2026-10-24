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
| M5 — Servicio e instalación | `netmon-tray service install/remove` con la unidad `systemd --user` (sesión gráfica, se relanza si falla salvo con otro motor en marcha, código 4), `netmon-tray version` del tag, README, licencia MIT, `go install` y release en GitHub: CI en cada push y, con cada tag `v*`, pruebas y binarios `linux-amd64` y `linux-arm64` con `SHA256SUMS` |
| M6 — Windows | Mismas funciones en Windows 10 (1803+) y 11 sin administrador: sondas con `IcmpSendEcho` y la tabla de rutas, bloqueo del historial con `LockFileEx`, contrato v1 por socket AF_UNIX en `%LOCALAPPDATA%\netmon-tray` con ACL solo del usuario y comprobación del SID del cliente, icono de bandeja, `service install/remove` con la clave `Run` de `HKCU`, log a fichero, binarios `netmon-tray.exe` y `netmon-trayw.exe` en `windows-amd64` y `windows-arm64`, y tests en `windows-latest` |

M1–M6 están hechos.

## Decisiones diferidas

| Decisión | Responsable | Último momento responsable |
| --- | --- | --- |
| Biblioteca de almacén del historial | Autor | Antes de escribir el almacén en M1 |
| Biblioteca de bandeja SNI | Autor | Antes de escribir el adaptador de icono en M1 |
| Objetivos, intervalos y umbrales por defecto de las sondas | Autor | Al medir consumo y detección en M1 |
| Umbrales numéricos de consumo y de detección | Autor | Al cerrar M1, tras medir |
| Canal de control y presentación en Windows | Autor | Decidido en M6: socket AF_UNIX e icono de bandeja |
