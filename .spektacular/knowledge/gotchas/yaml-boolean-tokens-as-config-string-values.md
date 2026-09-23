---
tags: [config, yaml, testing]
---

# A config value that is a YAML 1.1 boolean token needs a round-trip test

`off`, `on`, `yes` and `no` are boolean literals in YAML 1.1, so any config
enum that uses one of them as a string value sits on a type ambiguity. Today
`gopkg.in/yaml.v3` resolves it in our favour: a bare `off` decodes into a Go
`string` field as `"off"`, and marshalling quotes it back out. That is why
`auto_commit: off` works at all.

The trap is that nothing in the code says so. The behaviour depends on the
target field's type and on yaml.v3's resolution rules, not on anything we
declare, and if either changed the value would silently arrive as the boolean
`false` or as the empty string — the key would appear unset and the setting
would quietly fall back to its default. There would be no parse error to
notice.

So when adding a config key whose values include a YAML 1.1 boolean token,
pin the round trip with a test that asserts the **raw file text** on disk
(accepting either quoting, e.g. `auto_commit: "?off"?`) and that reading it
back yields the Go string. Asserting only the loaded value is not enough: it
passes just as well when the writer has silently changed the on-disk form.
`TestToYAMLFile_AutoCommitOffIsWrittenAsText` in
`internal/config/config_test.go` is the worked example.

The cheaper alternative is to avoid the token entirely — `disabled` rather
than `off` — where the naming is still open.
