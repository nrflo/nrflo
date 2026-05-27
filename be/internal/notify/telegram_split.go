package notify

import "strings"

// telegramMaxLen is Telegram's per-message limit, counted in UTF-16 code units.
const telegramMaxLen = 4096

// splitTelegram breaks a rendered MarkdownV2 body into chunks that each fit
// within telegramMaxLen UTF-16 code units. It prefers line boundaries so
// blockquotes and bold/link entities (which never span a newline in our
// templates) stay intact; an individual line longer than the limit is
// hard-split on rune boundaries without breaking a backslash escape pair.
func splitTelegram(body string) []string {
	if utf16Len(body) <= telegramMaxLen {
		return []string{body}
	}

	// Pre-split any oversized line so every segment fits on its own.
	var segs []string
	for _, line := range strings.Split(body, "\n") {
		if utf16Len(line) <= telegramMaxLen {
			segs = append(segs, line)
		} else {
			segs = append(segs, hardSplitLine(line, telegramMaxLen)...)
		}
	}

	// Greedily pack segments, rejoining with the newline they were split on.
	var chunks []string
	var cur strings.Builder
	curLen := 0
	for _, seg := range segs {
		segLen := utf16Len(seg)
		add := segLen
		if cur.Len() > 0 {
			add++ // the rejoining newline
		}
		if curLen+add > telegramMaxLen && cur.Len() > 0 {
			chunks = append(chunks, cur.String())
			cur.Reset()
			curLen = 0
		}
		if cur.Len() > 0 {
			cur.WriteByte('\n')
			curLen++
		}
		cur.WriteString(seg)
		curLen += segLen
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

// hardSplitLine splits a single line into pieces of at most max UTF-16 code
// units, never cutting between a backslash escape and the char it escapes.
func hardSplitLine(s string, max int) []string {
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		n, units := 0, 0
		for n < len(runes) {
			u := 1
			if runes[n] > 0xFFFF {
				u = 2
			}
			if units+u > max {
				break
			}
			units += u
			n++
		}
		if n == 0 {
			n = 1 // guarantee forward progress
		}
		if n < len(runes) && escapesNext(runes, n) {
			n--
			if n == 0 {
				n = 1
			}
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

// escapesNext reports whether the rune at index n is escaped by an odd-length
// run of backslashes immediately preceding it.
func escapesNext(runes []rune, n int) bool {
	bs := 0
	for i := n - 1; i >= 0 && runes[i] == '\\'; i-- {
		bs++
	}
	return bs%2 == 1
}

// utf16Len returns the length of s in UTF-16 code units, which is how Telegram
// counts message length.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}
