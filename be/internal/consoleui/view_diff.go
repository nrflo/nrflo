package consoleui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/pmezard/go-difflib/difflib"
)

var diffHunk = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func renderEditPreview(input map[string]json.RawMessage, fallback string) string {
	var changes []struct {
		Path string `json:"path"`
		Diff string `json:"diff"`
	}
	if json.Unmarshal(input["changes"], &changes) == nil && len(changes) > 0 {
		var blocks []string
		for _, change := range changes {
			blocks = append(blocks, toolTextStyle.Render(change.Path)+"\n"+renderDiff(change.Diff, change.Path))
		}
		return strings.Join(blocks, "\n")
	}
	path := inputText(input, "file_path", "filePath", "path")
	patch := inputText(input, "patch", "diff")
	if patch == "" {
		old, hasOld := input["old_string"]
		_, hasNew := input["new_string"]
		if hasOld && hasNew {
			var before string
			_ = json.Unmarshal(old, &before)
			after := inputText(input, "new_string")
			patch, _ = difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
				A: difflib.SplitLines(before), B: difflib.SplitLines(after), Context: 2,
			})
		} else if content := inputText(input, "content"); content != "" {
			patch = "@@ -0,0 +1 @@\n+" + strings.ReplaceAll(strings.TrimSuffix(content, "\n"), "\n", "\n+")
		}
	}
	if patch == "" && (strings.Contains(fallback, "@@ ") || strings.Contains(fallback, "*** Begin Patch")) {
		patch = fallback
	}
	if patch == "" {
		return renderStyledText(toolTextStyle, fallback)
	}
	header := path
	if header == "" {
		header = "patch"
	}
	return toolTextStyle.Render(header) + "\n" + renderDiff(patch, path)
}

func renderDiff(patch, path string) string {
	var out []string
	oldLine, newLine := 1, 1
	for _, line := range strings.Split(strings.TrimSuffix(expandTabs(previewText(patch)), "\n"), "\n") {
		if match := diffHunk.FindStringSubmatch(line); match != nil {
			oldLine, _ = strconv.Atoi(match[1])
			newLine, _ = strconv.Atoi(match[2])
			out = append(out, mutedStyle.Render(line))
			continue
		}
		if strings.HasPrefix(line, "*** ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "@@") {
			out = append(out, mutedStyle.Render(line))
			continue
		}
		if line == "" {
			out = append(out, "")
			continue
		}
		style := toolTextStyle
		var gutter string
		switch line[0] {
		case '+':
			gutter = fmt.Sprintf("%4d + ", newLine)
			newLine++
			style = style.Foreground(lipgloss.Color("114")).Background(lipgloss.Color("22"))
		case '-':
			gutter = fmt.Sprintf("%4d - ", oldLine)
			oldLine++
			style = style.Foreground(lipgloss.Color("203")).Background(lipgloss.Color("52"))
		case ' ':
			gutter = fmt.Sprintf("%4d   ", newLine)
			oldLine++
			newLine++
		default:
			out = append(out, mutedStyle.Render(line))
			continue
		}
		out = append(out, style.Render(gutter)+highlightSyntax(line[1:], path, style))
		if len(out) > toolPreviewRows*2 {
			out = append(out, mutedStyle.Render("… diff preview truncated"))
			break
		}
	}
	return strings.Join(out, "\n")
}
