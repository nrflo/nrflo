package consoleui

import (
	"strings"
	"unicode/utf8"
)

// cwdMaxRunes bounds the status bar's trailing cwd segment so it stays
// subordinate to the branch/YOLO segments that precede it.
const cwdMaxRunes = 28

// compactPath renders path for the status bar's cwd segment: the home prefix
// (if any) collapses to "~", and anything still over cwdMaxRunes elides its
// middle to "<first>/…/<second-to-last>/<last>". Rune-counted throughout so
// multi-byte path components truncate correctly.
func compactPath(path, home string) string {
	p := collapseHome(path, home)
	if utf8.RuneCountInString(p) <= cwdMaxRunes {
		return p
	}

	rooted := strings.HasPrefix(p, "/")
	rest := p
	if rooted {
		rest = rest[1:]
	}
	segs := strings.Split(rest, "/")
	if len(segs) < 4 {
		return elideRunes(p, cwdMaxRunes)
	}

	head := segs[0]
	if head == "~" && len(segs) > 1 {
		head += "/" + segs[1]
		segs = segs[1:]
	} else if rooted {
		head = "/" + head
	}
	tail := segs[len(segs)-2:]
	result := head + "/…/" + strings.Join(tail, "/")
	if utf8.RuneCountInString(result) > cwdMaxRunes {
		return elideRunes(result, cwdMaxRunes)
	}
	return result
}

// collapseHome replaces an exact or prefix match of home with "~"; a blank
// home (UserHomeDir failed) leaves path untouched.
func collapseHome(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

// elideRunes is the last-resort clamp: a rune-safe head truncation with a
// trailing ellipsis, used when the path has too few segments to elide by
// component (e.g. a single very long segment).
func elideRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}
