# Contrato de control v1

Contrato local entre el motor de netmon-tray y sus clientes (`netmon-tray ctl` y, en M4, el panel de Omarchy). Decisión: [ADR 0005](../architecture/decisions/0005-local-control-contract-for-panel-and-tray.md).

## Canal

- Socket Unix `$XDG_RUNTIME_DIR/netmon-tray/control.sock`. El directorio es `0700` y el socket `0600`.
- El motor comprueba el UID del par (`SO_PEERCRED`) en cada conexión y cierra sin responder la de otro usuario.
- Solo un motor es dueño del historial: lo garantiza un `flock` sobre `history.db.lock`, que el kernel suelta si el proceso muere. Un segundo motor termina con código 1 sin abrir el historial ni tocar el socket.
- El dueño reclama el socket después, bajo un segundo `flock` sobre `control.lock` en el mismo directorio que el socket, que mantiene hasta haberlo borrado al cerrar. Así se excluyen también motores con historiales distintos. Si otro motor tiene ese lock, responde en el socket o lo enlaza antes (`EADDRINUSE`), el nuevo termina con código 1. Solo un socket que rechaza la conexión (`ECONNREFUSED`) se considera huérfano y se sustituye; ante cualquier otro error se deja en su sitio.
- Sin `XDG_RUNTIME_DIR` o sin poder crear el socket, el motor sigue con la bandeja y registra un aviso.
- Hasta 16 conexiones a la vez; la 17.ª recibe `busy`.

## Mensajes

Cada conexión envía una línea JSON (máximo 4 KiB) y el motor responde con líneas JSON. `v` es la versión del contrato.

```json
{"v":1,"cmd":"status"}
```

| Orden | Respuesta |
| --- | --- |
| `status` | `{"v":1,"ok":true,"state":State}` y cierra |
| `watch` | `{"v":1,"ok":true}`, luego un `{"v":1,"event":"state","state":State}` con el estado actual y otro por cada cambio, hasta que el cliente o el motor cierran |
| `export` | `{"v":1,"ok":true,"files":["…csv","…json"]}` tras escribir el historial (hasta 30 s) |

`watch` solo emite cuando cambia el estado, la causa, o el último corte se abre, se cierra o cambia de causa; la duración de un corte abierto no genera eventos.

### State

| Campo | Significado |
| --- | --- |
| `state` | `unknown`, `up`, `degraded` o `down` |
| `cause` | `link`, `router` o `line`; ausente si no hay fallo |
| `last` | Último corte conocido; ausente si no hay ninguno |

`last`: `id`, `start`, `last_seen` (RFC 3339), `end` (ausente mientras está abierto), `duration_s`, `cause`, `open`.

### Errores

`{"v":1,"ok":false,"error":{"code":"…","message":"…"}}`. Los códigos son para máquinas y los mensajes (en español) para personas.

| Código | Cuándo |
| --- | --- |
| `bad_request` | La línea no es un objeto JSON o supera 4 KiB |
| `unsupported_version` | `v` distinto de 1 |
| `unknown_command` | `cmd` no existe |
| `busy` | Demasiadas conexiones |
| `export_failed` | El motor no pudo escribir los archivos |
| `export_unavailable` | El motor no tiene exportación configurada |

## Evolución

El contrato solo crece de forma compatible: campos y órdenes nuevos dentro de `v: 1`; un cambio incompatible sube `v`, y el motor rechaza con `unsupported_version` las que no habla. Los clientes ignoran los campos que no conocen.

## `netmon-tray ctl`

| Orden | Salida |
| --- | --- |
| `ctl status` | Texto en español; `--json` imprime el `State` en una línea |
| `ctl watch` | Una línea JSON por evento, tal como las emite el motor |
| `ctl export` | Una ruta por línea; `--json` imprime `{"files":[…]}` |

Códigos de salida: `0` aceptada, `1` rechazada o fallo del intercambio, `2` uso incorrecto, `3` motor no en marcha (también si `watch` lo pierde).
