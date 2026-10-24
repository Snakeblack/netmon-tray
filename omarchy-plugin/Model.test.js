// Offline tests of Model.js: node --test omarchy-plugin/
const test = require("node:test")
const assert = require("node:assert/strict")
const Model = require("./Model.js")

// Local times, so the expected texts do not depend on the time zone.
const start = new Date(2026, 9, 23, 21, 4, 5)
const end = new Date(2026, 9, 23, 21, 5, 10)

// A ctl watch line in the v1 wire form (docs/contract/control-v1.md).
function eventLine(state) {
  return JSON.stringify({ v: 1, event: "state", state: state })
}

const closedOutage = {
  id: 7, start: start.toISOString(), last_seen: end.toISOString(), end: end.toISOString(),
  duration_s: 65, cause: "router", open: false
}

test("parseEvent reads a v1 state event", () => {
  const state = Model.parseEvent(eventLine({ state: "down", cause: "line", last: closedOutage }))
  assert.equal(state.state, "down")
  assert.equal(state.cause, "line")
  assert.equal(state.last.id, 7)
})

test("parseEvent ignores what is not a v1 state event", () => {
  for (const line of ["", "no es json", "null", "[]", '{"v":1,"ok":true}', '{"v":2,"event":"state","state":{"state":"up"}}',
    '{"v":1,"event":"other","state":{"state":"up"}}', '{"v":1,"event":"state"}']) {
    assert.equal(Model.parseEvent(line), null, line)
  }
})

test("parseEvent turns an unknown state, a missing cause or a missing outage into safe values", () => {
  const state = Model.parseEvent(eventLine({ state: "rotten", extra: 1 }))
  assert.equal(state.state, "unknown")
  assert.equal(state.cause, "")
  assert.equal(state.last, null)
})

test("commandFrom uses the widget setting or netmon-tray", () => {
  assert.equal(Model.commandFrom({ command: " /opt/bin/netmon-tray " }), "/opt/bin/netmon-tray")
  for (const settings of [null, undefined, {}, { command: "" }, { command: "  " }, { command: 3 }]) {
    assert.equal(Model.commandFrom(settings), "netmon-tray")
  }
})

test("stateText names the state and, when it fails, the layer", () => {
  assert.equal(Model.stateText({ state: "up", cause: "" }), "Conectado")
  assert.equal(Model.stateText({ state: "degraded", cause: "" }), "Conexión degradada")
  assert.equal(Model.stateText({ state: "degraded", cause: "router" }), "Conexión degradada: falla el router")
  assert.equal(Model.stateText({ state: "down", cause: "link" }), "Sin conexión: falla el enlace local (wifi)")
  assert.equal(Model.stateText({ state: "down", cause: "line" }), "Sin conexión: falla la línea u operador")
  assert.equal(Model.stateText({ state: "down", cause: "" }), "Sin conexión: causa desconocida")
  assert.equal(Model.stateText({ state: "unknown", cause: "" }), "Sin datos")
})

test("barLook gives each state its own glyph, warns on failures and dims without an engine", () => {
  const glyphs = new Set()
  for (const s of ["up", "degraded", "down", "unknown"]) {
    const look = Model.barLook(true, { state: s, cause: "" })
    assert.equal(look.dimmed, false, s)
    assert.equal(look.urgent, s === "degraded" || s === "down", s)
    glyphs.add(look.glyph)
  }
  assert.equal(glyphs.size, 4)

  const off = Model.barLook(false, { state: "up", cause: "" })
  assert.equal(off.dimmed, true)
  assert.equal(off.urgent, false)
  assert.equal(off.tooltip, "netmon-tray no está en marcha")
  assert.equal(Model.barLook(true, null).dimmed, true)
  assert.equal(Model.barLook(true, { state: "down", cause: "router" }).tooltip, "Sin conexión: falla el router")
})

test("statusLine shows a failure first, then the engine and the state", () => {
  const up = { state: "up", cause: "" }
  assert.equal(Model.statusLine(true, up, "No se pudo exportar"), "No se pudo exportar")
  assert.equal(Model.statusLine(false, up, ""), "netmon-tray no está en marcha")
  assert.equal(Model.statusLine(true, null, ""), "Conectando…")
  assert.equal(Model.statusLine(true, up, ""), "Conectado")
})

test("formatTime writes day/month and the time of day", () => {
  assert.equal(Model.formatTime(start), "23/10 21:04:05")
  assert.equal(Model.formatTime(new Date(2026, 0, 2, 3, 4, 5)), "02/01 03:04:05")
  assert.equal(Model.formatTime(new Date("no es fecha")), "")
})

test("formatDuration is readable at every scale", () => {
  const cases = [
    [0, "0 s"], [5, "5 s"], [59, "59 s"], [60, "1 min"], [65, "1 min 5 s"], [3599, "59 min 59 s"],
    [3600, "1 h"], [3660, "1 h 1 min"], [3725, "1 h 2 min"], [86400, "1 d"], [90061, "1 d 1 h"],
    [-3, "0 s"], [4.7, "4 s"], [NaN, "0 s"]
  ]
  for (const [seconds, text] of cases) assert.equal(Model.formatDuration(seconds), text, String(seconds))
})

test("outageView describes a closed outage with the engine's duration", () => {
  const view = Model.outageView(closedOutage, end.getTime() + 3600 * 1000)
  assert.deepEqual(view, { open: false, when: "23/10 21:04:05", duration: "1 min 5 s", cause: "Falla el router" })
})

test("outageView counts an open outage from its start", () => {
  const open = { id: 8, start: start.toISOString(), last_seen: start.toISOString(), duration_s: 0, cause: "line", open: true }
  const view = Model.outageView(open, start.getTime() + 125 * 1000)
  assert.deepEqual(view, { open: true, when: "23/10 21:04:05", duration: "2 min 5 s", cause: "Falla la línea u operador" })
  // A clock behind the engine's never shows a negative duration.
  assert.equal(Model.outageView(open, start.getTime() - 5000).duration, "0 s")
})

test("outageView is null without an outage", () => {
  assert.equal(Model.outageView(null, Date.now()), null)
  assert.equal(Model.outageView(undefined, Date.now()), null)
})

test("ctl arguments", () => {
  assert.deepEqual(Model.watchArgs("nt"), ["nt", "ctl", "watch"])
  assert.deepEqual(Model.exportArgs("nt"), ["nt", "ctl", "export", "--json"])
  assert.deepEqual(Model.openFolderArgs("/d/exports"), ["xdg-open", "/d/exports"])
})

test("parseExport reads the files of ctl export --json and their folder", () => {
  const result = Model.parseExport('{"files":["/d/exports/h.csv","/d/exports/h.json"]}\n')
  assert.deepEqual(result, { files: ["/d/exports/h.csv", "/d/exports/h.json"], dir: "/d/exports" })
  for (const out of ["", "x", '{"files":[]}', '{"files":"a"}', '{"files":[3]}', "null"]) {
    assert.equal(Model.parseExport(out), null, out)
  }
})

test("ctlFailure explains each exit code", () => {
  assert.equal(Model.ctlFailure(3, "netmon-tray no está en marcha\n"), "netmon-tray no está en marcha")
  assert.equal(Model.ctlFailure(1, "\nNo se pudo exportar el historial (export_failed)\n"), "No se pudo exportar el historial (export_failed)")
  assert.equal(Model.ctlFailure(2, "Uso: netmon-tray ctl <orden>\n"), "Orden no válida para netmon-tray ctl: Uso: netmon-tray ctl <orden>")
  assert.equal(Model.ctlFailure(127, "sh: netmon-tray: not found"), "No se pudo ejecutar netmon-tray: sh: netmon-tray: not found")
  assert.equal(Model.ctlFailure(1, ""), "No se pudo ejecutar netmon-tray (código 1)")
})

test("missingCommand points to the widget setting", () => {
  assert.equal(Model.missingCommand("netmon-tray"), "No se puede ejecutar netmon-tray; pon su ruta absoluta en el ajuste «Comando de netmon-tray»")
})

test("startArgs starts the unit and, without it, the binary on its own", () => {
  const args = Model.startArgs("/opt/nt")
  assert.equal(args[0], "sh")
  assert.match(args[2], /systemctl --user start netmon-tray\.service/)
  assert.equal(args[3], "/opt/nt")
})
