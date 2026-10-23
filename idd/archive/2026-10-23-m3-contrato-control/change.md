# m3-contrato-control

## Intent and acceptance

El motor sirve un contrato de control local versionado v1 (status, watch, export) por socket Unix y el subcomando ctl lo consume

Acceptance: ctl status devuelve el estado y el último corte, ctl watch emite un evento por cada cambio de estado, ctl export escribe CSV/JSON y devuelve las rutas, y el socket rechaza a otros usuarios

## Plan

1. Adaptador `control`: protocolo v1 (`Request`, `Response`, `Event`, `State`, `Outage`), servidor sobre socket Unix (`Listen`/`Serve`/`Publish`/`Close`), cliente y `Ctl`.
2. Motor: `Engine.Export` serializa las exportaciones concurrentes (bandeja y ctl) y `app.Sinks` reparte cada estado a la bandeja y al socket.
3. `main`: subcomando `ctl`; el socket se reclama antes de abrir el historial.
4. Contrato documentado en `docs/contract/control-v1.md`.

## Decisions

- Confirmadas por el usuario («status + watch + export», «como alexa-tray», «permisos + SO_PEERCRED», «como alexa-tray»): órdenes `status`, `watch` y `export`; socket `$XDG_RUNTIME_DIR/netmon-tray/control.sock` con directorio 0700 y socket 0600; sin `XDG_RUNTIME_DIR` solo bandeja con aviso; otro motor que responde hace salir al nuevo con 1; un socket huérfano se borra; `ctl` es un subcomando del mismo binario con salida 0 aceptada, 1 rechazada, 2 uso y 3 motor no en marcha; `status` en texto con `--json`, `watch` en JSON por líneas, mensajes en español y códigos en inglés.
- `watch` solo emite cuando cambian el estado, la causa o el último corte (se abre, se cierra o cambia de causa): la duración de un corte abierto crece en cada muestra y no es un cambio.
- `Publish` nunca espera al cliente: cada suscriptor guarda solo el último estado pendiente.
- Los errores internos de exportación no llegan al cliente: `export_failed` lleva un mensaje genérico y el detalle va al registro.
- El socket se reclama antes de abrir el historial para que un segundo motor no toque el archivo (ADR 0003).
- Límite conocido (WARNING de la revisión de confianza, no bloqueante): el reclamo del socket no es atómico. Dos motores lanzados a la vez pueden competir; `EADDRINUSE` se trata hoy como fallo del socket y no como motor ya en marcha, y la comprobación de huérfano acepta cualquier error de conexión, no solo `ECONNREFUSED`. Pendiente de endurecer.
- La señal `persistent-data` salió de un `os.WriteFile` en una prueba; el cambio no persiste datos, y se satisfizo con las pruebas de migración existentes.
