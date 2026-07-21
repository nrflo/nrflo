package consoleui

// DECSET 1007 (alternate-scroll mode) makes terminals translate mouse-wheel
// events into arrow-key presses while the alternate screen buffer is active.
// Unlike DECSET 1000/1002/1003 mouse-tracking modes, it does not capture the
// mouse, so native click-drag text selection/copy keeps working. Bubble Tea
// v2 and x/ansi expose no first-class field/constant for mode 1007, so the
// TUI emits these raw sequences directly via tea.Raw.
const (
	altScrollEnable  = "\x1b[?1007h"
	altScrollDisable = "\x1b[?1007l"
)
