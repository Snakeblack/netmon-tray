// Package tray shows the connection state as a StatusNotifierItem icon.
package tray

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"time"

	"fyne.io/systray"

	"github.com/Snakeblack/netmon-tray/internal/app"
	"github.com/Snakeblack/netmon-tray/internal/domain"
)

const iconSize = 22

var (
	colorUp       = color.RGBA{R: 0x2e, G: 0xa0, B: 0x43, A: 0xff}
	colorDegraded = color.RGBA{R: 0xe5, G: 0xa0, B: 0x1a, A: 0xff}
	colorDown     = color.RGBA{R: 0xd7, G: 0x3a, B: 0x3a, A: 0xff}
	colorUnknown  = color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}
)

// Tray implements app.Sink.
type Tray struct {
	icons   map[domain.State][]byte
	status  *systray.MenuItem
	lastOne *systray.MenuItem
}

// New renders the four state icons.
func New() *Tray {
	return &Tray{icons: map[domain.State][]byte{
		domain.StateUp:       renderIcon(colorUp),
		domain.StateDegraded: renderIcon(colorDegraded),
		domain.StateDown:     renderIcon(colorDown),
		domain.StateUnknown:  renderIcon(colorUnknown),
	}}
}

// Run shows the icon and blocks until ctx is cancelled or the user picks
// "Salir". start runs the engine once the icon is ready.
func (t *Tray) Run(ctx context.Context, cancel context.CancelFunc, start func(ctx context.Context)) {
	systray.Run(func() {
		systray.SetIcon(t.icons[domain.StateUnknown])
		systray.SetTitle("netmon-tray")
		systray.SetTooltip("netmon-tray: sin datos")

		t.status = systray.AddMenuItem("Sin datos", "")
		t.status.Disable()
		t.lastOne = systray.AddMenuItem("Sin cortes registrados", "")
		t.lastOne.Disable()
		systray.AddSeparator()
		quit := systray.AddMenuItem("Salir", "Cierra netmon-tray")

		go start(ctx)
		go func() {
			select {
			case <-quit.ClickedCh:
				cancel()
			case <-ctx.Done():
			}
			systray.Quit()
		}()
	}, func() {})
}

// Publish updates icon, tooltip and menu. It never blocks on the network.
func (t *Tray) Publish(s app.Status) {
	if t.status == nil {
		return
	}
	icon := t.icons[s.State]
	if icon == nil {
		icon = t.icons[domain.StateUnknown]
	}
	systray.SetIcon(icon)

	text := StateText(s.Status)
	systray.SetTooltip("netmon-tray: " + text)
	t.status.SetTitle(text)
	t.lastOne.SetTitle(LastOutageText(s.Last))
}

// StateText is the Spanish description of the current status.
func StateText(s domain.Status) string {
	switch s.State {
	case domain.StateUp:
		return "Conectado"
	case domain.StateDegraded:
		return "Conexión degradada"
	case domain.StateDown:
		return "Sin conexión: " + CauseText(s.Cause)
	default:
		return "Sin datos"
	}
}

// CauseText names the failing layer in Spanish.
func CauseText(c domain.Cause) string {
	switch c {
	case domain.CauseLink:
		return "falla el enlace local (wifi)"
	case domain.CauseRouter:
		return "falla el router"
	case domain.CauseLine:
		return "falla la línea u operador"
	default:
		return "causa desconocida"
	}
}

// LastOutageText describes the last outage for the menu.
func LastOutageText(o *domain.Outage) string {
	if o == nil {
		return "Sin cortes registrados"
	}
	start := o.Start.Local().Format("02/01 15:04:05")
	if o.Open() {
		return fmt.Sprintf("Corte en curso desde %s", start)
	}
	return fmt.Sprintf("Último corte: %s, %s (%s)", start, o.Duration().Round(time.Second), CauseText(o.Cause))
}

func renderIcon(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	const r = iconSize/2 - 2
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			dx, dy := x-iconSize/2, y-iconSize/2
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
