# 0005. Contrato de control local para el panel y la bandeja

- Status: accepted
- Date: 2026-10-23

## Context and drivers
El panel de Omarchy debe mostrar el estado al día sin leer archivos ni sondear (`scenario:live-status`) y respetar el motor como único propietario de los datos (`constraint:architecture.data`). Esta decisión matiza el proceso único del ADR 0001: el panel es un cliente delgado del motor, no un segundo motor.

## Options considered
1. Socket local accesible solo para el usuario, con un contrato JSON versionado (estado, suscripción a cambios, órdenes), como en alexa-tray: interfaces desacopladas y sin puerto de red; añade un contrato que mantener.
2. El panel lee el archivo de historial: sin contrato, pero rompe la propiedad única de los datos.
3. Puerto HTTP local: portable, pero abre un puerto de escucha y contradice `constraint:architecture.trust`.

## Decision
El motor sirve un contrato de control local versionado por un canal accesible solo al usuario. El panel y las órdenes de línea de comandos lo consumen; el icono de bandeja vive en el propio motor.

## Consequences
El contrato solo crece de forma compatible. En Windows el canal puede ser distinto, tras el mismo puerto. Aceptado por el usuario el 2026-10-23.

## Fitness function
Una prueba de contrato comprueba que el motor emite cambios de estado y que el canal no acepta conexiones de otros usuarios.

## Review trigger
Si aparecen clientes que necesiten escribir, o si el canal local no es viable en Windows.
