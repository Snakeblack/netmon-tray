# netmon-tray

Monitor de cortes de red para Linux y [Windows](#windows). Vigila la conexión por capas (enlace, router, internet y DNS), registra cada corte con la capa que falla y guarda 2 meses de historial que puedes exportar a CSV y JSON para reclamar al operador. Muestra el estado en un icono de bandeja (StatusNotifierItem) y, en Omarchy, en su propio [panel de la barra](#panel-de-omarchy).

Funciona sin root, sin puertos de escucha y sin nube: todo se queda en tu equipo.

## Primer uso

Necesitas Linux con systemd y una sesión Wayland, y ping sin privilegios: tu grupo tiene que estar en `net.ipv4.ping_group_range` (la mayoría de distribuciones ya lo permiten a todos; compruébalo con `sysctl net.ipv4.ping_group_range`).

1. Instala el binario en `~/.local/bin` (u otra carpeta del `PATH`), de una de estas formas:

   - Desde [Releases](https://github.com/Snakeblack/netmon-tray/releases), con el `.tar.gz` de tu arquitectura (`linux-amd64` o `linux-arm64`):

     ```sh
     sha256sum -c --ignore-missing SHA256SUMS
     tar -xzf netmon-tray-v0.1.0-linux-amd64.tar.gz
     install -m 755 netmon-tray-v0.1.0-linux-amd64/netmon-tray ~/.local/bin/
     ```

   - Con Go:

     ```sh
     CGO_ENABLED=0 GOBIN=~/.local/bin go install github.com/Snakeblack/netmon-tray/cmd/netmon-tray@latest
     ```

     o, desde un clon del repositorio, `CGO_ENABLED=0 GOBIN=~/.local/bin go install ./cmd/netmon-tray`.

2. Arranca el motor con la sesión gráfica:

   ```sh
   netmon-tray service install
   ```

3. En Omarchy, instala el panel de la barra:

   ```sh
   netmon-tray plugin install
   ```

`netmon-tray service install` escribe la unidad `~/.config/systemd/user/netmon-tray.service`, que arranca la ruta del binario desde el que la instalas, la habilita con la sesión gráfica y la arranca. Si el motor ya estaba en marcha fuera de la unidad, la deja habilitada sin arrancarla y te dice cómo hacerlo: ciérralo con **Salir** en el icono y ejecuta `systemctl --user start netmon-tray`. Sin `systemctl`, o si falla, sale con 1 y muestra el error.

Con la unidad instalada, el motor se relanza solo a los 5 s si falla, pero no tras **Salir** ni si ya hay otro motor en marcha (sale con 4). **Iniciar** en el panel arranca la unidad y, sin ella, el binario suelto. `systemctl --user status netmon-tray` muestra su estado y `journalctl --user -u netmon-tray` su [registro](#registro).

Para actualizar, instala el binario nuevo como en el paso 1 y repite:

```sh
netmon-tray service install   # reinicia el motor con el binario nuevo
netmon-tray plugin install    # actualiza el panel si ha cambiado
```

`netmon-tray version` (o `--version`) muestra la versión. Go la toma del tag de git al compilar: `v0.1.0` en el commit del tag, una pseudo-versión (`v0.1.1-0.…-abcdef123456`) en los posteriores y `+dirty` si compilas con cambios sin guardar. El motor también la escribe en el registro al arrancar.

Para desinstalar: `netmon-tray service remove` para el motor, lo deshabilita y borra la unidad; `omarchy plugin remove snakeblack.netmon-tray` quita el panel, y `rm ~/.local/bin/netmon-tray` el binario. El historial y las exportaciones siguen en `~/.local/share/netmon-tray/`.

## Windows

En Windows hace lo mismo con otra presentación: el icono de la bandeja y su menú, sin panel. Funciona sin administrador en Windows 10 (1803 o posterior) y Windows 11.

1. Descarga de [Releases](https://github.com/Snakeblack/netmon-tray/releases) el `.zip` de tu arquitectura (`windows-amd64` o `windows-arm64`), comprueba su suma y descomprímelo en una carpeta tuya (cambia la versión por la que descargues):

   ```powershell
   (Get-FileHash netmon-tray-v0.2.0-windows-amd64.zip -Algorithm SHA256).Hash   # compárala con SHA256SUMS
   Expand-Archive netmon-tray-v0.2.0-windows-amd64.zip "$env:LOCALAPPDATA\Programs"
   cd "$env:LOCALAPPDATA\Programs\netmon-tray-v0.2.0-windows-amd64"
   ```

   Trae dos binarios: `netmon-tray.exe`, de consola, para `ctl`, `service` y `version`, y `netmon-trayw.exe`, el motor sin ventana de consola.

2. Arranca el motor con la sesión, desde esa carpeta:

   ```powershell
   .\netmon-tray.exe service install
   ```

`service install` registra `netmon-trayw.exe` en `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` (valor `netmon-tray`), que Windows lee al iniciar sesión, y lo arranca si no hay un motor en marcha. No necesita administrador ni crea un servicio de Windows: un servicio corre fuera de tu sesión y no podría mostrar el icono. A diferencia de la unidad de systemd, el motor no se relanza solo si falla; vuelve con la siguiente sesión. Si `netmon-trayw.exe` no está junto a `netmon-tray.exe`, sale con 1 y no registra nada.

Con Go, `go install` solo genera el binario de consola. Desde un clon del repositorio, compila los dos en la misma carpeta:

```powershell
go build -o netmon-tray.exe ./cmd/netmon-tray
go build -ldflags=-H=windowsgui -o netmon-trayw.exe ./cmd/netmon-tray
```

En Windows cambian estas rutas:

| | Windows |
| --- | --- |
| Historial y exportaciones | `%LOCALAPPDATA%\netmon-tray\` (`history.db` y `exports\`) |
| Socket de control | `%LOCALAPPDATA%\netmon-tray\control.sock` |
| Registro | `%LOCALAPPDATA%\netmon-tray\netmon-tray.log`; al arrancar, si pasa de 1 MiB, pasa a `netmon-tray.log.old` |

`ctl` funciona igual. El motor pone en `%LOCALAPPDATA%\netmon-tray` una ACL protegida solo para tu usuario y, además, rechaza cada conexión cuyo proceso sea de otro usuario. `netmon-tray plugin` no existe en Windows: sale con 2.

Para actualizar, sustituye los dos binarios tras cerrar el motor con **Salir** y repite `service install`. Para desinstalar, `.\netmon-tray.exe service remove` quita el registro de arranque; cierra el motor con **Salir** y borra la carpeta de los binarios. El historial sigue en `%LOCALAPPDATA%\netmon-tray\`.

## Cómo mide

Cada 2 s, el motor mide las capas de la más cercana a la más lejana. Solo mide una capa si las anteriores responden:

| Capa | Cómo se mide |
| --- | --- |
| Enlace | Hay ruta por defecto y su interfaz está activa (wifi o ethernet) |
| Router | Ping a la puerta de enlace de esa ruta (en Windows, con `IcmpSendEcho`, como `ping.exe`) |
| Internet | Ping a `1.1.1.1` y `8.8.8.8`; basta con que responda uno |
| DNS | Una consulta a un nombre aleatorio bajo `example.com`, como mucho cada 10 s, que el resolvedor no puede tener en caché |

Tras 3 sondeos seguidos con fallo (unos 6 s), el motor abre un corte y lo atribuye a la capa más cercana que falla: **enlace local**, **router** o **línea u operador**. Dos cortes separados por menos de 10 s cuentan como uno. Un DNS que falla con internet funcionando no es un corte, sino una conexión **degradada**, igual que los fallos sueltos que aún no llegan a corte.

Cada sondeo son unos pocos paquetes ICMP y, como mucho cada 10 s, una consulta DNS: no afecta a juegos ni a descargas.

## Icono y menú

El icono cambia de color con el estado: verde conectado, ámbar degradado, rojo sin conexión y gris mientras no hay datos. El tooltip y la primera línea del menú dicen el estado y, si falla algo, qué capa. Debajo está el último corte con su inicio, su duración y su causa o, si sigue abierto, desde cuándo.

- **Exportar historial** guarda el historial en CSV y JSON y muestra dónde.
- **Salir** cierra el motor. Con la unidad instalada, no se relanza hasta la siguiente sesión o hasta que pulses **Iniciar** en el panel.

## Historial y exportación

El historial vive en `~/.local/share/netmon-tray/history.db` (o `$XDG_DATA_HOME/netmon-tray/`) y guarda los cortes de los últimos 60 días; los anteriores se borran solos. Solo el motor lo escribe: si ya hay uno en marcha, el segundo no arranca.

Para reclamar al operador, exporta el historial desde el menú, desde el panel o con `netmon-tray ctl export`. Se escriben dos ficheros con la misma fecha en `~/.local/share/netmon-tray/exports/` (una carpeta solo para tu usuario):

- `netmon-historial-AAAAMMDD-HHMMSS.csv`, con las columnas `inicio`, `fin`, `duracion_s`, `causa` y `abierto`.
- `netmon-historial-AAAAMMDD-HHMMSS.json`, con un objeto por corte con esos mismos campos.

Las horas van en tu zona horaria (RFC 3339) y la causa es `link` (enlace local), `router` o `line` (línea u operador). Un corte que sigue abierto tiene `fin` vacío (`null` en JSON) y `abierto` a `true`.

## Control desde la terminal

Con el motor en marcha, `netmon-tray ctl` lo consulta, por ejemplo desde un script o un atajo de Hyprland:

```sh
netmon-tray ctl status          # estado de la conexión y último corte
netmon-tray ctl status --json
netmon-tray ctl watch           # una línea JSON con el estado en cada cambio
netmon-tray ctl export          # exporta el historial y muestra las rutas
netmon-tray ctl export --json
```

`ctl` sale con `0` si la orden se acepta, `1` si se rechaza o falla, `2` si el uso es incorrecto y `3` si el motor no está en marcha (también cuando un `watch` lo pierde).

El motor escucha en `$XDG_RUNTIME_DIR/netmon-tray/control.sock`, solo para tu usuario. El contrato está en [docs/contract/control-v1.md](docs/contract/control-v1.md).

## Panel de Omarchy

`omarchy-plugin/` es un plugin de omarchy-shell: un botón en la barra que abre un panel con el estado y el historial. El panel solo habla con el motor mediante `netmon-tray ctl`; nunca lee el historial.

- El icono de la barra cambia con el estado: conectado, degradado, sin conexión o sin datos. Degradado o sin conexión, se pone en color de aviso; sin motor, sale atenuado.
- **Último corte**: su inicio y su duración, que avanza en vivo mientras el corte sigue abierto, y la capa que falla.
- **Exportar historial** exporta a CSV y JSON y muestra la carpeta; **Abrir carpeta** la abre.
- Si el motor no está en marcha, el panel ofrece **Iniciar**, que arranca la unidad de `netmon-tray service install` o, sin ella, el binario. El panel vuelve a conectar solo cada 5 s.

El binario lleva el plugin dentro. Para instalarlo:

```sh
netmon-tray plugin install
```

Copia el plugin a `~/.config/omarchy/plugins/snakeblack.netmon-tray`, pide a omarchy-shell que lo descubra y pone el widget en la parte derecha de la barra. Si el widget ya está en la barra, no lo mueve. Como el panel sustituye al icono de bandeja, la primera vez también lo oculta en la bandeja de Omarchy. Si quieres volver a verlo, haz clic derecho en la flecha de la bandeja y pulsa **Show**: las siguientes instalaciones respetan esa elección (lo apuntan como `trayIconHidden` en la entrada del widget de `shell.json`). Repítelo tras actualizar `netmon-tray`: solo reescribe la copia si ha cambiado, y entonces `omarchy-restart-shell` carga el panel nuevo. Sin Omarchy, o con el shell parado, sale con 1; en el segundo caso la copia queda hecha y te dice cómo activarlo. Para quitarlo: `omarchy plugin remove snakeblack.netmon-tray`.

Para desarrollar el plugin, enlaza la carpeta del repositorio en lugar de copiarla; `netmon-tray plugin install` respeta el enlace y solo activa el widget:

```sh
ln -s "$PWD/omarchy-plugin" ~/.config/omarchy/plugins/snakeblack.netmon-tray
netmon-tray plugin install
```

El panel ejecuta `netmon-tray` del `PATH` del shell. Si está en otro sitio, pon su ruta absoluta en el ajuste **Comando de netmon-tray** del widget (`command` en `shell.json`); `netmon-tray plugin install` avisa si no lo encuentra en el `PATH`. Tras cambiar el QML de un plugin enlazado, `omarchy-restart-shell` lo vuelve a cargar. Un fichero nuevo del plugin va también en la línea `go:embed` de `omarchy-plugin/embed.go`; un test lo comprueba.

## Registro

El motor escribe una línea JSON por evento en stderr: el arranque (con la versión y la ruta del historial), cada corte, cada exportación y cada error. Con la unidad instalada, systemd lo guarda en el journal:

```sh
journalctl --user -u netmon-tray -f
```

## Desarrollo

```sh
go test ./...
go vet ./...
GOOS=windows go vet ./...
node --test omarchy-plugin/
```

Los tests son offline: las sondas y la bandeja quedan fuera, y el dominio se prueba con secuencias de sondeos simuladas. La lógica del panel vive en `omarchy-plugin/Model.js` y se prueba con node, sin Quickshell. El código de Windows se comprueba aquí con `GOOS=windows go vet ./...`, y sus tests corren en GitHub Actions en `windows-latest`. Un tag `v*` ejecuta los tests en Linux y en Windows y publica en Releases los binarios `linux-amd64`, `linux-arm64`, `windows-amd64` y `windows-arm64` con su `SHA256SUMS`.

## Licencia

[MIT](LICENSE)
