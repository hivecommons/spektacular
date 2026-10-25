package runview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// The markers in the page shell that Write replaces, each exactly once.
const (
	markerCSS  = "/*@CSS@*/"
	markerData = "@DATA@"
	markerJS   = "/*@JS@*/"
)

// dataOpen opens the element holding the page data.
const dataOpen = `<script type="application/json" id="run-data">`

// Write writes m as one self-contained HTML document: the embedded shell
// with the stylesheet, the script and m, serialised by encoding/json, spliced
// in. encoding/json escapes <, >, &, U+2028 and U+2029, so no string in the
// data can close the script element it sits in. The output depends on m
// alone: no generation time and no path.
func Write(w io.Writer, m Model) error {
	page, err := render(m)
	if err != nil {
		return err
	}
	_, err = w.Write(page)
	return err
}

func render(m Model) ([]byte, error) {
	shell, err := asset("page.html")
	if err != nil {
		return nil, err
	}
	css, err := asset("viewer.css")
	if err != nil {
		return nil, err
	}
	js, err := asset("viewer.js")
	if err != nil {
		return nil, err
	}
	for _, mk := range []string{markerCSS, markerData, markerJS} {
		if c := strings.Count(shell, mk); c != 1 {
			return nil, fmt.Errorf("runview: page shell has %d %q markers, want exactly 1", c, mk)
		}
	}
	if strings.Contains(strings.ToLower(css), "</style") {
		return nil, errors.New("runview: the stylesheet contains </style")
	}
	if strings.Contains(strings.ToLower(js), "</script") {
		return nil, errors.New("runview: the script contains </script")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("runview: encoding page data: %w", err)
	}
	// One pass, so text spliced in is never itself scanned for a marker.
	out := strings.NewReplacer(markerCSS, css, markerData, string(data), markerJS, js).Replace(shell)
	return []byte(out), nil
}

func asset(name string) (string, error) {
	b, err := assetFS.ReadFile("assets/" + name)
	if err != nil {
		return "", fmt.Errorf("runview: reading embedded %s: %w", name, err)
	}
	return string(b), nil
}

// Decode reads the page data back out of a page Write produced.
func Decode(page []byte) (Model, error) {
	var m Model
	i := bytes.Index(page, []byte(dataOpen))
	if i < 0 {
		return m, errors.New("runview: no run data in the page")
	}
	body := page[i+len(dataOpen):]
	j := bytes.Index(body, []byte("</script>"))
	if j < 0 {
		return m, errors.New("runview: the run data element is not closed")
	}
	if err := json.Unmarshal(body[:j], &m); err != nil {
		return m, fmt.Errorf("runview: decoding run data: %w", err)
	}
	return m, nil
}
