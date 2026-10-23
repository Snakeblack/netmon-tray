# 0002. Núcleo portable con adaptadores por sistema operativo

- Status: accepted
- Date: 2026-10-23

## Context and drivers
El producto es casi dedicado a Omarchy, pero debe ofrecer las mismas funciones en Windows aunque con otra presentación (`constraint:business.constraints`). Lo mantiene una sola persona, que ya usa el patrón hexagonal en un proyecto hermano (`constraint:team.shape`). La clasificación de cortes es el valor central y debe poder probarse sin red (`scenario:short-outage-detection`).

## Options considered
1. Núcleo de dominio y casos de uso independiente del sistema, con puertos para sondas, almacén y presentación, y adaptadores por sistema operativo: prueba offline y soporte futuro de Windows sin tocar el núcleo, a cambio de más estructura inicial.
2. Código acoplado a Linux con la portabilidad añadida después: más rápido al principio, pero obliga a reescribir al soportar Windows.
3. Dos bases de código, una por sistema: máxima libertad por plataforma, pero duplica el mantenimiento de una sola persona.

## Decision
El dominio (capas, sondas, clasificación, política de historial) no depende del sistema operativo ni de la interfaz. Bandeja, panel, servicio y detalles de las sondas viven en adaptadores por plataforma detrás de puertos.

## Consequences
El primer adaptador es Omarchy/Linux; Windows se añade como adaptadores nuevos sin cambiar el núcleo. Las sondas sin privilegios pueden diferir por sistema operativo y deben ocultarse tras el puerto de sondeo.

## Fitness function
Que el paquete de dominio no importe paquetes de adaptadores ni de bibliotecas de plataforma.
- Check: `! go list -deps ./internal/domain/... | grep -E 'internal/adapter|dbus|systemd'`

## Review trigger
Si una función de Windows no puede expresarse con los puertos existentes, o si el soporte de Windows se descarta.
