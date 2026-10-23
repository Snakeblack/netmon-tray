# 0003. Historial local con propietario único y esquema versionado

- Status: accepted
- Date: 2026-10-23

## Context and drivers
El historial de 2 meses debe sobrevivir a reinicios y poder exportarse como prueba al operador (`scenario:durable-history`). Hay un único usuario y no se usa nube (`constraint:functional.exclusions`). Las actualizaciones no deben perder datos (`constraint:delivery.versioning`).

## Options considered
1. Archivo local en el directorio de datos del usuario, con un único propietario (el motor), retención de 60 días y número de versión de esquema que migra al abrir: durable y sin servicios externos; exige disciplina de migración.
2. Solo en memoria con exportación manual: sin escritura a disco, pero se pierde en cada reinicio.
3. Servicio de base de datos externo: potente, pero contradice local, ligero y sin nube.

## Decision
Solo el motor escribe el historial, en un almacén local con versión de esquema; al abrir migra si hace falta. Los cortes más antiguos de 60 días se descartan. La exportación es una lectura del almacén.

## Consequences
Las interfaces obtienen el historial a través del motor, nunca del archivo. Cambiar el formato exige una migración con prueba. No hay copia automática: la exportación es la copia (`constraint:operation.backup`).

## Fitness function
Una prueba de integración reabre el almacén tras reinicio y tras migración, y comprueba la rotación a 60 días y la exportación.

## Review trigger
Si la retención sube de 2 meses, si el volumen de datos hace lenta la lectura, o si hace falta copia automática.
