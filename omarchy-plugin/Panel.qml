import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui
import "Model.js" as Model

// Bar button and panel of netmon-tray. The engine owns the history (ADR
// 0003): this panel only runs `netmon-tray ctl` (ADR 0005), keeps one `ctl
// watch` open for the live state and runs `ctl export` when asked.
Panel {
  id: root
  moduleName: "snakeblack.netmon-tray"
  ipcTarget: "snakeblack.netmon-tray"

  readonly property string command: Model.commandFrom(root.settings)
  // Last state the engine pushed, null until the first event.
  property var engineState: null
  // True while ctl watch delivers events.
  property bool running: false
  // Why watch is not running, from its last exit.
  property string watchFailure: ""
  // The folder of the last export, or why it failed.
  property string exportDir: ""
  property string exportFailure: ""
  // The clock an open outage counts from; it ticks only while it is shown.
  property real now: Date.now()

  readonly property var look: Model.barLook(running, engineState)
  readonly property string statusLine: Model.statusLine(running, engineState, running ? "" : watchFailure)
  readonly property var outage: running && engineState ? Model.outageView(engineState.last, now) : null

  readonly property string glyphPlay: String.fromCodePoint(0xf040a)
  readonly property string glyphExport: String.fromCodePoint(0xf095e)
  readonly property string glyphFolder: String.fromCodePoint(0xf0770)

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  function startEngine() {
    Quickshell.execDetached(Model.startArgs(command))
    startTimer.restart()
  }

  function exportHistory() {
    if (!running || exportProc.running) return
    exportDir = ""
    exportFailure = ""
    exportProc.command = Model.exportArgs(command)
    exportProc.running = true
  }

  function openFolder() {
    if (exportDir !== "") Quickshell.execDetached(Model.openFolderArgs(exportDir))
  }

  function restartWatch() {
    if (!watchProc.running) watchProc.running = true
  }

  onOpenedChanged: if (opened) now = Date.now()

  // A new command applies from the next watch: stopping it schedules the
  // retry, and a watch that is not running starts with it once the
  // process's own command binding has caught up.
  onCommandChanged: {
    if (watchProc.running) watchProc.running = false
    else Qt.callLater(restartWatch)
  }

  // A command that cannot start stops the process without exited, so the
  // retry is scheduled whenever it stops; launched tells the two apart.
  property bool watchLaunched: false

  Process {
    id: watchProc
    command: Model.watchArgs(root.command)
    running: true
    onStarted: root.watchLaunched = true
    onRunningChanged: {
      if (running) return
      root.running = false
      if (!root.watchLaunched) root.watchFailure = Model.missingCommand(root.command)
      root.watchLaunched = false
      retryTimer.restart()
    }
    stdout: SplitParser {
      onRead: function(line) {
        var state = Model.parseEvent(line)
        if (!state) return
        root.engineState = state
        root.running = true
        root.now = Date.now()
      }
    }
    stderr: StdioCollector { id: watchStderr }
    onExited: function(exitCode) {
      root.watchFailure = Model.ctlFailure(exitCode, watchStderr.text)
    }
  }

  Process {
    id: exportProc
    stdout: StdioCollector { id: exportStdout }
    stderr: StdioCollector { id: exportStderr }
    onExited: function(exitCode) {
      var result = exitCode === 0 ? Model.parseExport(exportStdout.text) : null
      if (result) root.exportDir = result.dir
      else root.exportFailure = exitCode === 0 ? "netmon-tray no devolvió ningún archivo" : Model.ctlFailure(exitCode, exportStderr.text)
    }
  }

  // Without an engine, watch is retried every 5 s.
  Timer { id: retryTimer; interval: 5000; onTriggered: root.restartWatch() }
  // After Iniciar, the engine needs a moment to open its socket.
  Timer { id: startTimer; interval: 1500; onTriggered: { retryTimer.stop(); root.restartWatch() } }
  // The duration of an open outage, while the panel shows it.
  Timer {
    interval: 1000
    repeat: true
    running: root.opened && root.outage !== null && root.outage.open
    onTriggered: root.now = Date.now()
  }

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: String.fromCodePoint(root.look.glyph)
    dimmed: root.look.dimmed
    active: root.look.urgent
    tooltipText: root.look.tooltip
    onPressed: function(b) {
      if (b === Qt.LeftButton) root.toggle()
    }
  }

  KeyboardPanel {
    id: panel
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(360))
    contentHeight: panel.fittedContentHeight(column.implicitHeight)

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }
      onActivateRequested: if (!root.running) root.startEngine()

      Column {
        id: column
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        spacing: Style.space(12)

        // ---------- Hero: state glyph · title and state · Iniciar ----------
        Item {
          width: parent.width
          implicitHeight: Math.max(heroIcon.implicitHeight, heroLabels.implicitHeight, heroAction.implicitHeight)

          Text {
            id: heroIcon
            textFormat: Text.PlainText
            text: String.fromCodePoint(root.look.glyph)
            color: root.look.urgent ? root.bar.urgent : root.bar.foreground
            opacity: root.look.dimmed ? 0.5 : 1
            font.family: root.bar.fontFamily
            font.pixelSize: Style.font.display
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
          }

          Column {
            id: heroLabels
            anchors.left: heroIcon.right
            anchors.leftMargin: Style.space(14)
            anchors.right: heroAction.visible ? heroAction.left : parent.right
            anchors.rightMargin: Style.space(10)
            anchors.verticalCenter: parent.verticalCenter
            spacing: Style.space(2)

            Text {
              textFormat: Text.PlainText
              text: "Cortes de red"
              color: root.bar.foreground
              font.family: root.bar.fontFamily
              font.pixelSize: Style.font.title
              font.bold: true
            }

            Text {
              textFormat: Text.PlainText
              text: root.statusLine
              color: root.look.urgent ? root.bar.urgent : Qt.darker(root.bar.foreground, 1.4)
              font.family: root.bar.fontFamily
              font.pixelSize: Style.font.bodySmall
              wrapMode: Text.Wrap
              width: parent.width
            }
          }

          Button {
            id: heroAction
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            visible: !root.running
            iconText: root.glyphPlay
            text: "Iniciar"
            fontSize: Style.font.bodySmall
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
            bordered: true
            onClicked: root.startEngine()
          }
        }

        // ---------- Last outage: when, how long and where it failed ----------
        Column {
          visible: root.running && !!root.engineState
          width: parent.width
          spacing: Style.space(4)

          PanelSectionHeader {
            text: "ÚLTIMO CORTE"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
          }

          BodyText {
            visible: root.outage === null
            text: "Sin cortes registrados"
            opacity: 0.6
          }

          BodyText {
            visible: root.outage !== null
            text: root.outage === null ? "" : (root.outage.open ? "En curso desde " + root.outage.when : root.outage.when)
            color: root.outage !== null && root.outage.open ? root.bar.urgent : root.bar.foreground
          }

          BodyText {
            visible: root.outage !== null
            text: root.outage === null ? "" : "Duración: " + root.outage.duration + " · " + root.outage.cause
            opacity: 0.7
          }
        }

        // ---------- History: export it as proof for the operator ----------
        PanelSeparator {
          visible: root.running && !!root.engineState
          foreground: root.bar.foreground
        }

        Column {
          visible: root.running && !!root.engineState
          width: parent.width
          spacing: Style.space(8)

          PanelSectionHeader {
            text: "HISTORIAL"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
          }

          Row {
            spacing: Style.space(6)

            Button {
              enabled: !exportProc.running
              opacity: enabled ? 1 : 0.5
              iconText: root.glyphExport
              text: exportProc.running ? "Exportando…" : "Exportar historial"
              tooltipText: "Guarda los últimos 60 días en CSV y JSON"
              fontSize: Style.font.bodySmall
              foreground: root.bar.foreground
              fontFamily: root.bar.fontFamily
              bordered: true
              onClicked: root.exportHistory()
            }

            Button {
              visible: root.exportDir !== ""
              iconText: root.glyphFolder
              text: "Abrir carpeta"
              fontSize: Style.font.bodySmall
              foreground: root.bar.foreground
              fontFamily: root.bar.fontFamily
              bordered: true
              onClicked: root.openFolder()
            }
          }

          // Selectable, so the folder can be copied.
          TextEdit {
            visible: root.exportDir !== ""
            textFormat: TextEdit.PlainText
            text: "Exportado en " + root.exportDir
            readOnly: true
            selectByMouse: true
            color: root.bar.foreground
            font.family: root.bar.fontFamily
            font.pixelSize: Style.font.bodySmall
            wrapMode: TextEdit.WrapAnywhere
            width: parent.width
          }

          BodyText {
            visible: root.exportFailure !== ""
            text: root.exportFailure
            color: root.bar.urgent
          }
        }
      }
    }
  }

  component BodyText: Text {
    textFormat: Text.PlainText
    color: root.bar.foreground
    font.family: root.bar.fontFamily
    font.pixelSize: Style.font.bodySmall
    wrapMode: Text.Wrap
    width: parent ? parent.width : 0
  }
}
