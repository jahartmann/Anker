package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type setupDisplay struct {
	out             io.Writer
	animated, color bool
	width           int
}

func newSetupDisplay() setupDisplay {
	tty := term.IsTerminal(os.Stdout.Fd()) && os.Getenv("TERM") != "dumb"
	width := 68
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
		width = max(1, min(68, w-2))
	}
	_, noColor := os.LookupEnv("NO_COLOR")
	return setupDisplay{out: os.Stdout, color: tty && !noColor, animated: tty && width >= 20 && os.Getenv("ANKER_NO_ANIMATION") != "1", width: width}
}

func setupDisplayText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 127 {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

func (d setupDisplay) style(code, text string) string {
	if !d.color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (d setupDisplay) Header() {
	fmt.Fprintln(d.out, "\n"+d.style("1", "Anker")+"  "+d.style("90", "Servereinrichtung"))
	fmt.Fprintln(d.out, d.style("90", strings.Repeat("─", d.width)))
	fmt.Fprintln(d.out, d.style("90", "Zugang  /  Verbindung  /  Prüfen & speichern"))
	fmt.Fprintln(d.out, "Enter übernimmt die Vorgabe in [Klammern]. Ctrl+C bricht ab.")
}

func (d setupDisplay) Section(number, title string) {
	fmt.Fprintln(d.out, "\n"+d.style("34", number)+"  "+d.style("1", title))
}

func (d setupDisplay) Note(text string) {
	fmt.Fprintln(d.out, d.style("90", setupDisplayText(text)))
}

func (d setupDisplay) Fact(label, value string) {
	fmt.Fprintln(d.out, "  "+d.style("90", fmt.Sprintf("%-16s", label))+" "+setupDisplayText(value))
}

func (d setupDisplay) Prompt(label, fallback string) string {
	prompt := d.style("1", setupDisplayText(label))
	if fallback != "" {
		prompt += d.style("90", " ["+setupDisplayText(fallback)+"]")
	}
	// Keep the final colon plain, including in color terminals and PTY scripts.
	return prompt + ": "
}

func (d setupDisplay) Wait(label string, task func() error) error {
	label = setupDisplayText(label)
	if !d.animated {
		fmt.Fprintln(d.out, "  · "+label+" …")
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	if d.animated {
		go func() {
			defer close(stopped)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			for frame := 0; ; frame++ {
				fmt.Fprint(d.out, "\r\x1b[2K  "+d.style("34", frames[frame%len(frames)])+" "+ansi.Truncate(label, max(1, d.width-6), "…"))
				select {
				case <-stop:
					return
				case <-ticker.C:
				}
			}
		}()
	}
	err := func() error {
		if d.animated {
			defer func() { close(stop); <-stopped }()
		}
		return task()
	}()
	if d.animated {
		fmt.Fprint(d.out, "\r\x1b[2K")
	}
	if err != nil {
		fmt.Fprintln(d.out, "  "+d.style("31", "×")+" "+label+" · fehlgeschlagen")
	} else {
		fmt.Fprintln(d.out, "  "+d.style("32", "✓")+" "+label+" · bereit")
	}
	return err
}
