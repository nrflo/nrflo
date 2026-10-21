package consoleui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Highlight only persisted tool text; never execute commands or read file paths.
func highlightSyntax(text, filename string, base lipgloss.Style) string {
	text = expandTabs(previewText(text))
	lexer := lexers.Match(filename)
	if lexer == nil {
		return renderStyledText(base, text)
	}
	iter, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, text)
	if err != nil {
		return renderStyledText(base, text)
	}
	var out strings.Builder
	for token := iter(); token != chroma.EOF; token = iter() {
		style := base
		switch {
		case token.Type.InCategory(chroma.Keyword):
			style = style.Foreground(lipgloss.Color("141"))
		case token.Type.InSubCategory(chroma.LiteralString):
			style = style.Foreground(lipgloss.Color("114"))
		case token.Type.InSubCategory(chroma.NameVariable):
			style = style.Foreground(lipgloss.Color("213"))
		case token.Type.InCategory(chroma.Name):
			style = style.Foreground(lipgloss.Color("81"))
		case token.Type.InCategory(chroma.Operator):
			style = style.Foreground(lipgloss.Color("214"))
		case token.Type.InCategory(chroma.Comment):
			style = style.Foreground(muted)
		case token.Type.InSubCategory(chroma.LiteralNumber):
			style = style.Foreground(lipgloss.Color("180"))
		}
		out.WriteString(renderStyledText(style, token.Value))
	}
	return out.String()
}

// Lipgloss pads multiline strings to equal width; tool text must keep its spacing.
func renderStyledText(style lipgloss.Style, text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = style.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
