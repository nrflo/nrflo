package notify

import "strings"

const (
	// telegramMaxLen is Telegram's per-message hard API limit, counted in
	// UTF-16 code units. A sendMessage above this is rejected outright.
	telegramMaxLen = 4096

	// telegramChunkTarget is the soft per-message target. It sits below the
	// mobile-client "Show more" cutoff so every chunk renders fully expanded
	// instead of collapsing behind a tap-to-expand link.
	telegramChunkTarget = 3000
)

// splitTelegram breaks a rendered MarkdownV2 body into chunks that each fit
// within telegramChunkTarget (soft) and never exceed telegramMaxLen (hard).
// It splits at paragraph boundaries (blank lines, including blockquote-prefixed
// "> " lines) first, then falls back to line boundaries inside any paragraph
// that's still too big, then hard-splits an oversized line on rune boundaries
// without breaking a MarkdownV2 backslash escape pair.
//
// Blank-line atoms that land on a chunk boundary are dropped — the seam between
// two messages already separates the paragraphs visually.
func splitTelegram(body string) []string {
	if utf16Len(body) <= telegramChunkTarget {
		return []string{body}
	}

	paras := splitParagraphs(body)

	var chunks []string
	var cur strings.Builder
	curLen := 0
	const sep = "\n\n"
	sepLen := 2

	flush := func() {
		if cur.Len() > 0 {
			chunks = append(chunks, cur.String())
			cur.Reset()
			curLen = 0
		}
	}

	for _, p := range paras {
		pLen := utf16Len(p)
		if pLen > telegramChunkTarget {
			// Paragraph alone exceeds the target — flush current and line-pack
			// this paragraph into its own chunk(s).
			flush()
			chunks = append(chunks, linePackParagraph(p)...)
			continue
		}
		add := pLen
		if cur.Len() > 0 {
			add += sepLen
		}
		if curLen+add > telegramChunkTarget && cur.Len() > 0 {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteString(sep)
			curLen += sepLen
		}
		cur.WriteString(p)
		curLen += pLen
	}
	flush()
	return chunks
}

// splitParagraphs groups consecutive non-blank lines into paragraphs. A blank
// line (empty, "> ", or ">") is a separator and is dropped. Each returned
// paragraph is the lines joined with "\n".
func splitParagraphs(body string) []string {
	var out []string
	var cur []string
	for _, line := range strings.Split(body, "\n") {
		if isBlankLine(line) {
			if len(cur) > 0 {
				out = append(out, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, "\n"))
	}
	return out
}

// isBlankLine reports whether a line is empty or a bare blockquote marker —
// the patterns that act as paragraph separators in our rendered bodies.
func isBlankLine(s string) bool {
	t := strings.TrimRight(s, " \t")
	return t == "" || t == ">"
}

// linePackParagraph packs the lines of a single paragraph into chunks that
// fit within telegramChunkTarget (soft) and never exceed telegramMaxLen
// (hard). A line longer than telegramMaxLen is hard-split on rune boundaries.
func linePackParagraph(p string) []string {
	var atoms []string
	for _, line := range strings.Split(p, "\n") {
		if utf16Len(line) <= telegramMaxLen {
			atoms = append(atoms, line)
		} else {
			atoms = append(atoms, hardSplitLine(line, telegramMaxLen)...)
		}
	}

	var chunks []string
	var cur strings.Builder
	curLen := 0
	flush := func() {
		if cur.Len() > 0 {
			chunks = append(chunks, cur.String())
			cur.Reset()
			curLen = 0
		}
	}
	for _, a := range atoms {
		aLen := utf16Len(a)
		add := aLen
		if cur.Len() > 0 {
			add++ // rejoining newline
		}
		if curLen+add > telegramChunkTarget && cur.Len() > 0 {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteByte('\n')
			curLen++
		}
		cur.WriteString(a)
		curLen += aLen
	}
	flush()
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
