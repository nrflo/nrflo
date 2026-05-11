package spec_import

import (
	"fmt"
	"strings"
)

// adfNode is the minimal shape of an Atlassian Document Format node.
type adfNode struct {
	Type    string                 `json:"type"`
	Text    string                 `json:"text,omitempty"`
	Content []adfNode              `json:"content,omitempty"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
	Marks   []adfMark              `json:"marks,omitempty"`
}

type adfMark struct {
	Type string `json:"type"`
}

// adfToText converts an ADF document node to plain text.
func adfToText(node adfNode) string {
	var b strings.Builder
	walkADF(&b, node, 0)
	return strings.TrimSpace(b.String())
}

func walkADF(b *strings.Builder, node adfNode, listDepth int) {
	switch node.Type {
	case "doc":
		for _, child := range node.Content {
			walkADF(b, child, listDepth)
		}

	case "paragraph":
		for _, child := range node.Content {
			walkADF(b, child, listDepth)
		}
		b.WriteString("\n\n")

	case "heading":
		level := 1
		if v, ok := node.Attrs["level"]; ok {
			switch n := v.(type) {
			case float64:
				level = int(n)
			case int:
				level = n
			}
		}
		b.WriteString(strings.Repeat("#", level) + " ")
		for _, child := range node.Content {
			walkADF(b, child, listDepth)
		}
		b.WriteString("\n\n")

	case "bulletList":
		for _, child := range node.Content {
			walkADF(b, child, listDepth+1)
		}

	case "orderedList":
		idx := 1
		for _, child := range node.Content {
			if child.Type == "listItem" {
				b.WriteString(fmt.Sprintf("%d. ", idx))
				idx++
				for _, c := range child.Content {
					walkADF(b, c, listDepth+1)
				}
				b.WriteString("\n")
			}
		}

	case "listItem":
		b.WriteString("- ")
		for _, child := range node.Content {
			walkADF(b, child, listDepth)
		}
		b.WriteString("\n")

	case "codeBlock":
		lang := ""
		if v, ok := node.Attrs["language"]; ok {
			if s, ok := v.(string); ok {
				lang = s
			}
		}
		b.WriteString("```" + lang + "\n")
		for _, child := range node.Content {
			walkADF(b, child, listDepth)
		}
		b.WriteString("```\n\n")

	case "text":
		txt := node.Text
		isCode := false
		for _, mark := range node.Marks {
			if mark.Type == "code" {
				isCode = true
			}
		}
		if isCode {
			b.WriteString("`" + txt + "`")
		} else {
			b.WriteString(txt)
		}

	case "inlineCard", "mention":
		if url, ok := node.Attrs["url"]; ok {
			b.WriteString(fmt.Sprintf("%v", url))
		}

	case "hardBreak":
		b.WriteString("\n")

	default:
		// Unknown node: recurse into content to avoid dropping text.
		for _, child := range node.Content {
			walkADF(b, child, listDepth)
		}
	}
}
