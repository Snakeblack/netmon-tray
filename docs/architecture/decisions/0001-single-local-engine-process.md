# 0001. Motor local de proceso único en espacio de usuario

- Status: accepted
- Date: 2026-10-23

## Context and drivers
Un único desarrollador y operador local (`constraint:team.shape`, `constraint:team.production-owner`) necesita vigilar la conexión sin afectar a juegos ni a la red (`scenario:minimal-footprint`) y mantener el estado siempre al día (`scenario:live-status`), sin infraestructura intermedia (`constraint:architecture.boundaries`).

## Options considered
1. Un único proceso de usuario que sondea, clasifica, guarda el historial y sirve el estado: mínimo consumo y despliegue trivial, a cambio de que el historial solo se registre mientras el proceso esté vivo.
2. Demonio de fondo más cliente de bandeja separado por IPC: registra aunque la bandeja no corra, pero duplica binarios y contratos para un único usuario.
3. Servicio en contenedor local con panel web: reutiliza clientes, pero contradice el objetivo de ligereza.

## Decision
Un solo proceso de usuario, el motor, es el único que sondea, clasifica y escribe el historial. Las interfaces (icono y panel) no hacen E/S de red ni leen el historial directamente.

## Consequences
El despliegue es un ejecutable gestionado por la sesión del usuario. Sin el motor en marcha no se registran cortes, así que el arranque automático es parte del producto. Queda prohibido que una interfaz sondee la red por su cuenta.

## Fitness function
Revisión de que existe un único punto de entrada de proceso y de que ningún manejador de interfaz realiza E/S de red síncrona.

## Review trigger
Si hay que registrar cortes mientras no haya sesión gráfica, o si varios procesos cliente necesitan escribir.
