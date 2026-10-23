# Brief de producto — netmon-tray

## Problema

La conexión a internet sufre cortes de pocos segundos que no se pueden explicar: no se sabe si falla el wifi, el router, la línea o la red del operador, y no hay pruebas objetivas para reclamar al operador. (fuente: usuario)

## Usuarios

- El propio autor, que lo usa y lo opera en su PC (Linux con Omarchy).
- Otros usuarios de escritorio de Omarchy que lo instalen desde GitHub (`Snakeblack/netmon-tray`) y, en segundo lugar, usuarios de Windows.

## Objetivos

| Prioridad | Objetivo | Medida |
| --- | --- | --- |
| 1 | Consumo mínimo: no afectar a juegos ni a la conexión | Sin cifra confirmada; se mide al cerrar el primer slice |
| 2 | Detectar cortes de pocos segundos y clasificar dónde fallan (wifi, router, línea, operador) | Sin cifra confirmada |
| 3 | Pruebas para el operador: historial de 2 meses exportable que sobreviva a reinicios | 60 días de retención, exportación CSV/JSON |
| 4 | Panel e icono siempre al día con el estado real, como alexa-tray | Sin cifra confirmada |

## No objetivos

- Test de velocidad o medición de ancho de banda.
- Servicios en la nube, subida de datos o telemetría.
- Alertas externas (correo, mensajería).
- Multiusuario o administración remota.

## Restricciones

- Dedicado casi por completo a Omarchy, con un panel propio igual que alexa-tray.
- Debe funcionar también en Windows con las mismas funciones, aunque no con la misma presentación.
- Un solo mantenedor (perfil senior). Licencia MIT, solo dependencias permisivas.
- Proceso de usuario sin root y sin puerto de escucha en red.
