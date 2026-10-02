---
tags: [template, mustache, escaping]
---

# Mustache `{{.}}` HTML-escapes what it renders

Step templates and skills are rendered with `cbroglie/mustache`, and its double-brace tags
(`{{name}}`, `{{.}}`) HTML-escape their value. That is harmless for names and prose, but it
corrupts a URL: a source link like `https://tracker.example/item?a=1&b=2` reaches the agent as
`...?a=1&amp;b=2`, and the agent then fetches the wrong address.

Render any value the agent will use verbatim, such as a URL, a query string or a shell
fragment, with the triple-brace form, which does not escape:

```mustache
{{#sources}}
- `{{{.}}}`
{{/sources}}
```

This was found when the interview step and the finished step's chaining offer listed an epic's
source links with `{{.}}`. Both now use `{{{.}}}`, and a step test checks that a link containing
`&` comes through unchanged. Apply the same check to any new template that renders a link.
