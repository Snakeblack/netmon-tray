# Glosario — netmon-tray

| Término | Definición en este proyecto |
| --- | --- |
| **Corte** | Intervalo en el que la conectividad deja de ser válida, de pocos segundos o más. Se registra con inicio, duración y clasificación. |
| **Capa** | Punto de la ruta que se sondea por separado: enlace local (wifi), router, línea y red del operador / internet. |
| **Sonda** | Comprobación activa y ligera (ICMP, DNS o TCP) a un objetivo de una capa. |
| **Clasificación** | Atribución de un corte a la capa más cercana al usuario que falla. |
| **Historial** | Registro local de cortes y estados con retención de 60 días y esquema versionado. |
| **Exportación** | Volcado del historial a CSV/JSON para entregarlo como prueba al operador. |
| **Motor** | Proceso único que sondea, clasifica, guarda el historial y sirve el estado a la interfaz. |
| **SNI (StatusNotifierItem)** | Especificación D-Bus con la que Omarchy y otras barras muestran el icono y el menú de bandeja. |
| **Panel de Omarchy** | Plugin QML de omarchy-shell que muestra el estado y el último corte (hito posterior). |
