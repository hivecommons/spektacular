package jira

import (
	"regexp"
	"strings"
)

// The bodies publish renders are a small, regular Markdown: ATX headings,
// paragraphs, "- " bullets (some as "- [ ] " checklist items), fenced code
// blocks, and inline `code`, **strong** and [text](url) links. This file
// converts that subset to the two description formats Jira accepts: the
// Atlassian Document Format for Cloud's REST v3, and wiki markup for Data
// Center's REST v2. Anything the subset does not cover is kept as text.

var (
	headingRE = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	bulletRE  = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	inlineRE  = regexp.MustCompile("(`[^`]+`)|(\\*\\*[^*]+\\*\\*)|(\\[[^\\]]+\\]\\([^)]+\\))")
	linkRE    = regexp.MustCompile(`^\[([^\]]+)\]\(([^)]+)\)$`)
)

type block struct {
	kind  string // heading, paragraph, bullets, code
	level int
	text  string
	items []string
}

// parseBlocks splits Markdown into blocks.
func parseBlocks(md string) []block {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var blocks []block
	var para []string
	flushPara := func() {
		if len(para) > 0 {
			blocks = append(blocks, block{kind: "paragraph", text: strings.Join(para, " ")})
			para = nil
		}
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			flushPara()
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			blocks = append(blocks, block{kind: "code", text: strings.Join(code, "\n")})
		case trimmed == "":
			flushPara()
		case headingRE.MatchString(trimmed):
			flushPara()
			m := headingRE.FindStringSubmatch(trimmed)
			blocks = append(blocks, block{kind: "heading", level: len(m[1]), text: m[2]})
		case bulletRE.MatchString(line):
			flushPara()
			b := block{kind: "bullets"}
			for ; i < len(lines) && bulletRE.MatchString(lines[i]); i++ {
				b.items = append(b.items, bulletRE.FindStringSubmatch(lines[i])[1])
			}
			i--
			blocks = append(blocks, b)
		default:
			para = append(para, trimmed)
		}
	}
	flushPara()
	return blocks
}

// adfNode is one node of an Atlassian Document.
type adfNode map[string]any

// toADF converts Markdown to an ADF document.
func toADF(md string) adfNode {
	content := []adfNode{}
	for _, b := range parseBlocks(md) {
		switch b.kind {
		case "heading":
			content = append(content, adfNode{"type": "heading", "attrs": map[string]any{"level": b.level}, "content": adfInline(b.text)})
		case "paragraph":
			content = append(content, adfNode{"type": "paragraph", "content": adfInline(b.text)})
		case "code":
			text := b.text
			if text == "" {
				text = " "
			}
			content = append(content, adfNode{"type": "codeBlock", "content": []adfNode{{"type": "text", "text": text}}})
		case "bullets":
			items := []adfNode{}
			for _, it := range b.items {
				items = append(items, adfNode{"type": "listItem", "content": []adfNode{{"type": "paragraph", "content": adfInline(it)}}})
			}
			content = append(content, adfNode{"type": "bulletList", "content": items})
		}
	}
	if len(content) == 0 {
		content = append(content, adfNode{"type": "paragraph", "content": []adfNode{{"type": "text", "text": " "}}})
	}
	return adfNode{"type": "doc", "version": 1, "content": content}
}

// adfInline renders a line with inline code, strong and link marks.
func adfInline(text string) []adfNode {
	var out []adfNode
	add := func(s string, marks ...adfNode) {
		if s == "" {
			return
		}
		n := adfNode{"type": "text", "text": s}
		if len(marks) > 0 {
			n["marks"] = marks
		}
		out = append(out, n)
	}
	last := 0
	for _, loc := range inlineRE.FindAllStringIndex(text, -1) {
		add(text[last:loc[0]])
		tok := text[loc[0]:loc[1]]
		switch {
		case strings.HasPrefix(tok, "`"):
			add(strings.Trim(tok, "`"), adfNode{"type": "code"})
		case strings.HasPrefix(tok, "**"):
			add(strings.Trim(tok, "*"), adfNode{"type": "strong"})
		default:
			m := linkRE.FindStringSubmatch(tok)
			add(m[1], adfNode{"type": "link", "attrs": map[string]any{"href": m[2]}})
		}
		last = loc[1]
	}
	add(text[last:])
	if len(out) == 0 {
		add(" ")
	}
	return out
}

// ADFText flattens an ADF document (as decoded JSON) to its text, one line
// per block, so a marker written into a description can be read back.
func ADFText(doc any) string {
	var b strings.Builder
	var walk func(n any)
	walk = func(n any) {
		m, ok := n.(map[string]any)
		if !ok {
			return
		}
		if m["type"] == "text" {
			if s, ok := m["text"].(string); ok {
				b.WriteString(s)
			}
			return
		}
		if kids, ok := m["content"].([]any); ok {
			for _, k := range kids {
				walk(k)
			}
		}
		switch m["type"] {
		case "paragraph", "heading", "codeBlock", "listItem":
			b.WriteString("\n")
		}
	}
	walk(doc)
	return b.String()
}

// toWiki converts Markdown to Jira wiki markup.
func toWiki(md string) string {
	var b strings.Builder
	for _, blk := range parseBlocks(md) {
		switch blk.kind {
		case "heading":
			b.WriteString("h" + string(rune('0'+blk.level)) + ". " + wikiInline(blk.text) + "\n\n")
		case "paragraph":
			b.WriteString(wikiInline(blk.text) + "\n\n")
		case "code":
			b.WriteString("{code}\n" + blk.text + "\n{code}\n\n")
		case "bullets":
			for _, it := range blk.items {
				b.WriteString("* " + wikiInline(it) + "\n")
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func wikiInline(text string) string {
	return inlineRE.ReplaceAllStringFunc(text, func(tok string) string {
		switch {
		case strings.HasPrefix(tok, "`"):
			return "{{" + strings.Trim(tok, "`") + "}}"
		case strings.HasPrefix(tok, "**"):
			return "*" + strings.Trim(tok, "*") + "*"
		default:
			m := linkRE.FindStringSubmatch(tok)
			return "[" + m[1] + "|" + m[2] + "]"
		}
	})
}

// jiraLabel is a label as Jira accepts it: no whitespace.
func jiraLabel(l string) string {
	return strings.Join(strings.Fields(l), "-")
}
