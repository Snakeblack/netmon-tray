# m3-reclamo-socket

## Intent and acceptance

Dos motores lanzados a la vez pueden compartir el historial o dejar sin socket al primero: el reclamo del socket no es atómico, EADDRINUSE no cuenta como motor en marcha y cualquier error de conexión se toma como socket huérfano.

Acceptance: con dos motores lanzados a la vez sobre los mismos datos, exactamente uno abre el historial y sirve el socket y el otro sale con 1 sin abrirlo; un error de conexión distinto de ECONNREFUSED o ENOENT nunca borra el socket.

## Plan

1. Almacén: `storage.Open` toma un `flock` exclusivo no bloqueante sobre `history.db.lock` antes de tocar el archivo y lo suelta en `Close`; si está tomado devuelve `ErrInUse`.
2. Control: `Listen` toma un `flock` sobre `control.lock` en el directorio del socket y lo mantiene hasta que `Close` ha borrado el socket; `EADDRINUSE` es `ErrAlreadyRunning`; solo `ECONNREFUSED` borra un socket huérfano.
3. Control: el bucle de `accept` espera con backoff (de 5 ms a 1 s) ante un fallo persistente y `Close` lo corta al instante.
4. `main`: abre el historial antes de reclamar el socket; contrato actualizado.

## Decisions

- Confirmadas por el usuario («flock en el historial», «sigue solo con bandeja»): la exclusión del historial es un `flock` sobre `history.db.lock` que el kernel suelta si el proceso muere; si el motor tiene el historial pero no puede crear el socket, sigue solo con bandeja y aviso, y nunca borra un socket ajeno.
- Corrige el WARNING de la revisión de confianza de m3-contrato-control (reclamo del socket no atómico).
- `storage.Open` cerraba los cortes abiertos «de un proceso anterior»: sin lock, un segundo motor habría cerrado el corte en curso del primero. Por eso el lock va en el almacén y se toma antes de abrir SQLite.
- Segundo `flock` en el directorio de control (WARNING de la primera revisión de este cambio): dos motores con historiales distintos no comparten el lock del historial y podían borrarse el socket entre sí. El lock se suelta después de borrar el socket, para que nadie vea el propio como huérfano.
- Backoff en `accept` (SUGGESTION de la misma revisión): un `EMFILE` persistente hacía girar el bucle sin pausa (8,8 millones de reintentos en 200 ms en la prueba).
- `lock.go` del almacén es solo `unix`, como el resto del control (`SO_PEERCRED`); Windows queda para M6.
- Los archivos `history.db.lock` y `control.lock` quedan vacíos tras parar el motor; son inocuos.
