# Technical baseline — netmon-tray

## Context and boundaries

Herramienta local de escritorio, dedicada a Omarchy y con Windows como segundo objetivo. Un usuario, sin servicios propios ni nube.

| Elemento | Detalle |
| --- | --- |
| Actores | Usuario de escritorio; el operador de internet solo recibe la exportación |
| Sistemas externos | Objetivos de red sondeados (router, DNS, internet), barra de Omarchy (SNI y plugin QML) |
| Fuera de alcance | Test de velocidad, nube, telemetría, alertas externas, multiusuario |

## Components and data

| Componente | Responsabilidad | Datos que posee |
| --- | --- | --- |
| Dominio | Capas, clasificación de cortes, política de historial | Reglas; no persiste |
| Motor | Orquesta sondeo, clasificación y almacén; sirve el estado (ADR 0001) | Historial y configuración |
| Adaptador de sondas | Ejecuta las sondas por capa sin privilegios (ADR 0004) | Ninguno |
| Almacén de historial | Archivo local versionado con retención de 60 días (ADR 0003) | Cortes y estados; único escritor: el motor |
| Contrato de control | Estado, suscripción y órdenes para las interfaces (ADR 0005) | Ninguno |
| Adaptadores de presentación | Icono SNI y panel de Omarchy; en Windows, el icono de la bandeja con su menú (ADR 0002) | Ninguno |

## Integrations

| Dependencia | Propietario | Protocolo | Modo de fallo |
| --- | --- | --- | --- |
| Objetivos de sondeo | Operador / terceros | ICMP, DNS, TCP | Sin respuesta es la señal medida; se distingue de un fallo local por capas |
| Barra de Omarchy | Omarchy | StatusNotifierItem sobre D-Bus; plugin QML | Sin barra, el motor sigue registrando; la interfaz se degrada |
| Servicio de usuario | systemd --user (Linux) | Unidad de usuario | Sin arranque automático no hay registro; se relanza si falla |
| Arranque con la sesión | Windows | Clave `Run` de `HKCU` con `netmon-trayw.exe` | Sin arranque automático no hay registro; no se relanza si falla |

## Quality scenarios

| Id | Atributo | Estímulo | Respuesta | Medida | Prioridad |
| --- | --- | --- | --- | --- | --- |
| `minimal-footprint` | Eficiencia | Motor en ejecución continua durante el juego | Poca CPU y RAM, tráfico casi nulo | Sin cifra confirmada; se mide en el primer slice | 1 |
| `short-outage-detection` | Fiabilidad de detección | Caída de pocos segundos | Registra el corte y lo clasifica por capa | Sin cifra confirmada | 2 |
| `durable-history` | Durabilidad | Reinicio o petición de exportar | Conserva 2 meses, migra el esquema y exporta | 60 días de retención | 3 |
| `live-status` | Capacidad de respuesta | Cambia el estado de la red | Icono y panel al día sin bloquear | Sin cifra confirmada | 4 |

## Constraints

| Restricción | Fuente |
| --- | --- |
| Mantenedor único, perfil senior | Usuario (`team.shape`) |
| Lo opera el autor y lo publica en GitHub para otros usuarios de Omarchy | Usuario (`team.production-owner`) |
| Omarchy primero; Windows con las mismas funciones aunque otra presentación | Usuario (`business.constraints`) |
| Proceso de usuario sin root, sin telemetría, sin puerto de escucha | Usuario (`architecture.trust`) |
| Licencia MIT y solo dependencias permisivas | Usuario (`technology.licenses`) |
| Logs locales estructurados, sin métricas ni trazas externas | Usuario (`operation.observability`) |
| Sin copia automática; la exportación es la copia | Usuario (`operation.backup`) |

## Test strategy

| Capa | Qué protege |
| --- | --- |
| Unitarias | Clasificación de cortes con secuencias simuladas, retención y reglas de dominio |
| Integración | Almacén (reapertura, migración, rotación, exportación) y contrato de control |
| Medición | Consumo de CPU/RAM y bytes por sondeo, sin umbral hasta que el usuario lo confirme |

No se fija porcentaje de cobertura.

## Technology record

| Elección | Implementa | Alternativas | Por qué | Fuente |
| --- | --- | --- | --- | --- |
| Go | ADR 0002 | Rust | Experiencia del autor, binario estático, compila para Linux y Windows y reutiliza los patrones de alexa-tray | 2026-10-23 — respuesta del usuario en foundation |
| Licencia MIT | ADR 0002 | GPL-3.0 | Repositorio público, sin condicionar dependencias | 2026-10-23 — respuesta del usuario en foundation |
| systemd --user, SNI y plugin QML | ADR 0001, ADR 0005 | Paquete AUR | Mismo despliegue que alexa-tray, sin empaquetado adicional | 2026-10-23 — respuesta del usuario en foundation |
| GitHub Actions y Releases | ADR 0002 | Release manual | Pruebas automáticas antes de publicar | 2026-10-23 — respuesta del usuario en foundation |

La elección concreta de la biblioteca de almacén y de la biblioteca de bandeja se difiere al primer slice (ver `docs/roadmap.md`).
