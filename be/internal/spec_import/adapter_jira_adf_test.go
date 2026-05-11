package spec_import

import (
	"strings"
	"testing"
)

func doc(children ...adfNode) adfNode {
	return adfNode{Type: "doc", Content: children}
}

func para(children ...adfNode) adfNode {
	return adfNode{Type: "paragraph", Content: children}
}

func textNode(s string) adfNode {
	return adfNode{Type: "text", Text: s}
}

func codeTextNode(s string) adfNode {
	return adfNode{Type: "text", Text: s, Marks: []adfMark{{Type: "code"}}}
}

func heading(level int, children ...adfNode) adfNode {
	return adfNode{
		Type:    "heading",
		Attrs:   map[string]interface{}{"level": float64(level)},
		Content: children,
	}
}

func bulletList(items ...adfNode) adfNode {
	return adfNode{Type: "bulletList", Content: items}
}

func listItem(children ...adfNode) adfNode {
	return adfNode{Type: "listItem", Content: children}
}

func orderedList(items ...adfNode) adfNode {
	return adfNode{Type: "orderedList", Content: items}
}

func codeBlock(lang string, children ...adfNode) adfNode {
	return adfNode{
		Type:    "codeBlock",
		Attrs:   map[string]interface{}{"language": lang},
		Content: children,
	}
}

func TestADFToText_Paragraph(t *testing.T) {
	n := doc(para(textNode("Hello paragraph")))
	got := adfToText(n)
	if !strings.Contains(got, "Hello paragraph") {
		t.Errorf("adfToText = %q, want to contain %q", got, "Hello paragraph")
	}
}

func TestADFToText_Heading(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "# "},
		{2, "## "},
		{3, "### "},
	}
	for _, tc := range cases {
		n := doc(heading(tc.level, textNode("Title")))
		got := adfToText(n)
		if !strings.Contains(got, tc.want+"Title") {
			t.Errorf("heading level %d: got %q, want to contain %q", tc.level, got, tc.want+"Title")
		}
	}
}

func TestADFToText_BulletList(t *testing.T) {
	n := doc(bulletList(
		listItem(para(textNode("item one"))),
		listItem(para(textNode("item two"))),
	))
	got := adfToText(n)
	if !strings.Contains(got, "- ") {
		t.Errorf("bullet list: got %q, want to contain '- '", got)
	}
	if !strings.Contains(got, "item one") {
		t.Errorf("bullet list: got %q, want 'item one'", got)
	}
	if !strings.Contains(got, "item two") {
		t.Errorf("bullet list: got %q, want 'item two'", got)
	}
}

func TestADFToText_OrderedList(t *testing.T) {
	n := doc(orderedList(
		listItem(para(textNode("first"))),
		listItem(para(textNode("second"))),
	))
	got := adfToText(n)
	if !strings.Contains(got, "1.") {
		t.Errorf("ordered list: got %q, want '1.'", got)
	}
	if !strings.Contains(got, "2.") {
		t.Errorf("ordered list: got %q, want '2.'", got)
	}
}

func TestADFToText_CodeBlock(t *testing.T) {
	n := doc(codeBlock("go", textNode("fmt.Println(\"hello\")")))
	got := adfToText(n)
	if !strings.Contains(got, "```go") {
		t.Errorf("codeBlock: got %q, want to contain '```go'", got)
	}
	if !strings.Contains(got, "fmt.Println") {
		t.Errorf("codeBlock: got %q, want code content", got)
	}
	if !strings.Contains(got, "```") {
		t.Errorf("codeBlock: got %q, want closing ```", got)
	}
}

func TestADFToText_CodeBlock_NoLang(t *testing.T) {
	n := doc(codeBlock("", textNode("x := 1")))
	got := adfToText(n)
	if !strings.HasPrefix(strings.TrimSpace(got), "```") {
		t.Errorf("codeBlock no lang: got %q, want to start with ```", got)
	}
}

func TestADFToText_InlineCode(t *testing.T) {
	n := doc(para(codeTextNode("myVar")))
	got := adfToText(n)
	if !strings.Contains(got, "`myVar`") {
		t.Errorf("inline code: got %q, want '`myVar`'", got)
	}
}

func TestADFToText_HardBreak(t *testing.T) {
	n := doc(para(textNode("line1"), adfNode{Type: "hardBreak"}, textNode("line2")))
	got := adfToText(n)
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("hardBreak: got %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("hardBreak: expected newline in %q", got)
	}
}

func TestADFToText_NilContent_NoPanic(t *testing.T) {
	// Nodes with nil Content slices must not panic.
	n := adfNode{Type: "doc", Content: nil}
	got := adfToText(n)
	if got != "" {
		t.Errorf("empty doc: got %q, want empty", got)
	}
}

func TestADFToText_UnknownNodeRecurses(t *testing.T) {
	n := doc(adfNode{
		Type: "unknownNodeType",
		Content: []adfNode{
			para(textNode("inner text")),
		},
	})
	got := adfToText(n)
	if !strings.Contains(got, "inner text") {
		t.Errorf("unknown node: got %q, want 'inner text'", got)
	}
}
