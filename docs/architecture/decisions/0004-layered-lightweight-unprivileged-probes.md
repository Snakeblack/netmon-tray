# 0004. Sondas activas por capas, ligeras y sin privilegios

- Status: accepted
- Date: 2026-10-23

## Context and drivers
Hay que detectar cortes de pocos segundos y atribuirlos a wifi, router, línea u operador (`scenario:short-outage-detection`) sin cargar la CPU ni la red (`scenario:minimal-footprint`) y sin root ni puertos de escucha (`constraint:architecture.trust`).

## Options considered
1. Sondas activas de coste mínimo a un objetivo por capa (enlace local, router, internet) con ICMP sin privilegios, DNS o TCP según el sistema: distinguen capas y casi no generan tráfico.
2. Lectura pasiva del estado del sistema: sin tráfico, pero no detecta cortes de línea u operador.
3. ICMP crudo con capacidad elevada: más preciso, pero exige privilegios y complica la instalación.

## Decision
El motor sondea cada capa con sondas ligeras que se ejecutan sin privilegios y clasifica el corte en la capa más cercana al usuario que falla. Intervalos, objetivos y umbrales son configurables y los valores por defecto se fijan al medir.

## Consequences
La precisión depende de la elección de objetivos y de la frecuencia de sondeo, y se ajusta con medición real. Los únicos paquetes salientes son las sondas; no se abre ningún puerto en red.

## Fitness function
Pruebas unitarias del clasificador con resultados simulados por capa, incluido un corte de pocos segundos, y revisión de que ninguna sonda requiere privilegios.

## Review trigger
Si las sondas sin privilegios no distinguen cortes de pocos segundos con fiabilidad, o si Windows exige otro mecanismo.
