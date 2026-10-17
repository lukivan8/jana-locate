package main

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

type linkMouseScreen struct {
	tcell.SimulationScreen
	calls []bool
}

func (s *linkMouseScreen) EnableMouse(flags ...tcell.MouseFlags) {
	s.calls = append(s.calls, true)
	s.SimulationScreen.EnableMouse(flags...)
}
func (s *linkMouseScreen) DisableMouse() {
	s.calls = append(s.calls, false)
	s.SimulationScreen.DisableMouse()
}

func TestWebURLHyperlinkWrapScrollAndMouseOwnership(t *testing.T) {
	u, _, _ := uiFixture(t)
	screen := &linkMouseScreen{SimulationScreen: u.screen.(tcell.SimulationScreen)}
	u.screen = screen
	u.normalize = true
	target := u.site + "?run=" + u.run.Id
	expected := u.style(tcell.ColorLightBlue, false).Underline(true).Url(target).UrlId("jana-run-" + u.run.Id)
	for _, width := range []int{120, 39} {
		screen.SetSize(width, 24)
		u.mode = "url"
		u.detailScroll = 0
		u.render()
		fragments := wrapLines(target, width-4)
		for i, line := range fragments {
			x := 2
			for _, r := range line {
				got, _, style, _ := screen.GetContent(x, 8+i)
				if got != r || style != expected {
					t.Fatalf("URL fragment %d col %d lost complete link: %q", i, x, got)
				}
				x++
			}
		}
		_, _, style, _ := screen.GetContent(2, 8+len(fragments)+1)
		if style == expected {
			t.Fatal("link leaked into instructions")
		}
		// Scrolling retains the same URL target, even when its first line is hidden.
		u.detailScroll = 1
		screen.SetSize(width, 14)
		u.render()
		if len(fragments) > 1 {
			_, _, style, _ = screen.GetContent(2, 8)
			if style != expected {
				t.Fatal("scrolled fragment lost link")
			}
		}
		u.handle(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
		u.render()
	}
	if len(screen.calls) != 4 || screen.calls[0] || !screen.calls[1] || screen.calls[2] || !screen.calls[3] {
		t.Fatalf("mouse ownership not restored: %v", screen.calls)
	}
}
