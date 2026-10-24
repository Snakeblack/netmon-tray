// Pure logic of the netmon-tray panel, shared by Panel.qml and the node tests.
// The panel only talks to the engine through `netmon-tray ctl` (ADR 0005), so
// everything here builds ctl arguments or reads what ctl prints.

var contractVersion = 1
// ctl exit codes, as docs/contract/control-v1.md fixes them.
var exitRejected = 1
var exitUsage = 2
var exitNotRunning = 3

var states = ["unknown", "up", "degraded", "down"]

// One glyph per state, so the bar does not tell them apart by color alone.
var stateGlyphs = {
  up: 0xf0789,       // md-web_check
  degraded: 0xf124a, // md-web_clock
  down: 0xf0a8e,     // md-web_off
  unknown: 0xf059f   // md-web
}

var causeTexts = {
  link: "falla el enlace local (wifi)",
  router: "falla el router",
  line: "falla la línea u operador"
}

// The command that runs netmon-tray: the widget setting, or netmon-tray from PATH.
function commandFrom(settings) {
  var value = settings && typeof settings.command === "string" ? settings.command.trim() : ""
  return value !== "" ? value : "netmon-tray"
}

// The state carried by one line of `ctl watch`, or null when the line is not
// a v1 state event. An unknown state reads as "unknown", a missing cause as
// "" and a missing outage as null, so the panel never meets undefined.
function parseEvent(line) {
  var event
  try {
    event = JSON.parse(String(line || ""))
  } catch (e) {
    return null
  }
  if (!event || event.v !== contractVersion || event.event !== "state" || !event.state || typeof event.state !== "object") return null
  var s = event.state
  return {
    state: states.indexOf(s.state) >= 0 ? s.state : "unknown",
    cause: typeof s.cause === "string" ? s.cause : "",
    last: s.last && typeof s.last === "object" ? s.last : null
  }
}

function causeText(cause) {
  return causeTexts[cause] || "causa desconocida"
}

function capitalize(text) {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

// The state in Spanish, as `ctl status` writes it; a degraded state with a
// failing layer also names it.
function stateText(state) {
  switch (state.state) {
  case "up":
    return "Conectado"
  case "degraded":
    return state.cause ? "Conexión degradada: " + causeText(state.cause) : "Conexión degradada"
  case "down":
    return "Sin conexión: " + causeText(state.cause)
  default:
    return "Sin datos"
  }
}

// How the bar button looks: its glyph, dimmed without an engine, urgent when
// the network fails, and the tooltip.
function barLook(running, state) {
  if (!running || !state) {
    return { glyph: stateGlyphs.unknown, dimmed: true, urgent: false, tooltip: running ? "Conectando…" : "netmon-tray no está en marcha" }
  }
  return {
    glyph: stateGlyphs[state.state] || stateGlyphs.unknown,
    dimmed: false,
    urgent: state.state === "degraded" || state.state === "down",
    tooltip: stateText(state)
  }
}

// The status line under the panel title. A failed order wins until the
// next state event clears it.
function statusLine(running, state, failure) {
  if (failure) return failure
  if (!running) return "netmon-tray no está en marcha"
  if (!state) return "Conectando…"
  return stateText(state)
}

function pad(n) {
  return n < 10 ? "0" + n : String(n)
}

// Local day/month and time of day, as `ctl status` writes them.
function formatTime(date) {
  if (!(date instanceof Date) || isNaN(date.getTime())) return ""
  return pad(date.getDate()) + "/" + pad(date.getMonth() + 1) + " " + pad(date.getHours()) + ":" + pad(date.getMinutes()) + ":" + pad(date.getSeconds())
}

// A duration in whole seconds with its two largest units: "45 s",
// "1 min 5 s", "2 h 3 min", "1 d 4 h". A zero second unit is left out.
function formatDuration(seconds) {
  var total = Math.floor(Number(seconds))
  if (!(total > 0)) return "0 s"
  var units = [["d", 86400], ["h", 3600], ["min", 60], ["s", 1]]
  for (var i = 0; i < units.length; i++) {
    var big = Math.floor(total / units[i][1])
    if (big === 0) continue
    var text = big + " " + units[i][0]
    if (i + 1 < units.length) {
      var small = Math.floor((total % units[i][1]) / units[i + 1][1])
      if (small > 0) text += " " + small + " " + units[i + 1][0]
    }
    return text
  }
  return "0 s"
}

// The last outage for the panel, or null without one. An open outage counts
// from its start to now, since the engine sends no event while it lasts; a
// closed one shows the duration the engine recorded.
function outageView(last, nowMs) {
  if (!last) return null
  var start = new Date(last.start)
  var seconds = last.open ? (nowMs - start.getTime()) / 1000 : last.duration_s
  return {
    open: last.open === true,
    when: formatTime(start),
    duration: formatDuration(seconds),
    cause: capitalize(causeText(last.cause))
  }
}

function watchArgs(command) {
  return [command, "ctl", "watch"]
}

function exportArgs(command) {
  return [command, "ctl", "export", "--json"]
}

// The files `ctl export --json` printed and their folder, or null when the
// output is not a non-empty list of paths.
function parseExport(stdout) {
  var result
  try {
    result = JSON.parse(String(stdout || ""))
  } catch (e) {
    return null
  }
  if (!result || !Array.isArray(result.files) || result.files.length === 0) return null
  for (var i = 0; i < result.files.length; i++) {
    if (typeof result.files[i] !== "string" || result.files[i] === "") return null
  }
  var first = result.files[0]
  var slash = first.lastIndexOf("/")
  return { files: result.files, dir: slash > 0 ? first.slice(0, slash) : "/" }
}

function openFolderArgs(dir) {
  return ["xdg-open", dir]
}

// What a failed ctl run means for people. A missing binary or an
// unexpected code says so.
function ctlFailure(exitCode, stderr) {
  if (exitCode === exitNotRunning) return "netmon-tray no está en marcha"
  var lines = String(stderr || "").split("\n").filter(function(l) { return l.trim() !== "" })
  var text = lines.length > 0 ? lines[0].trim() : ""
  if (exitCode === exitRejected && text !== "") return text
  if (exitCode === exitUsage) return "Orden no válida para netmon-tray ctl" + (text !== "" ? ": " + text : "")
  if (text !== "") return "No se pudo ejecutar netmon-tray: " + text
  return "No se pudo ejecutar netmon-tray (código " + exitCode + ")"
}

// Why watch never ran: the shell could not start the command.
function missingCommand(command) {
  return "No se puede ejecutar " + command + "; pon su ruta absoluta en el ajuste «Comando de netmon-tray»"
}

// Iniciar starts the systemd --user unit and, without it (or without
// systemctl), the engine binary on its own.
function startArgs(command) {
  return ["sh", "-c", "systemctl --user start netmon-tray.service 2>/dev/null || exec \"$0\"", command]
}

if (typeof module !== "undefined") {
  module.exports = {
    commandFrom: commandFrom,
    parseEvent: parseEvent,
    causeText: causeText,
    stateText: stateText,
    barLook: barLook,
    statusLine: statusLine,
    formatTime: formatTime,
    formatDuration: formatDuration,
    outageView: outageView,
    watchArgs: watchArgs,
    exportArgs: exportArgs,
    parseExport: parseExport,
    openFolderArgs: openFolderArgs,
    ctlFailure: ctlFailure,
    missingCommand: missingCommand,
    startArgs: startArgs
  }
}
