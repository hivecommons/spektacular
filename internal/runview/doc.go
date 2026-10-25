// Package runview turns one recorded workflow run (an internal/runstats Run)
// into a self-contained HTML waterfall page.
//
// It has two layers. Build is pure: from a run and the run statistics
// report it builds the compact, columnar Model the page draws, with every
// rule about what a bar means (tree order, effective ends, problem flags,
// gaps, the longest steps) decided here, where Go tests cover it. Write
// joins the embedded page shell, stylesheet and script with the model,
// serialised as HTML-safe JSON, into one document on an io.Writer. The page
// script only draws what it is given.
//
// The package never reads or touches the logs directory: it is handed a
// loaded run, and its output carries no file path and no generation time,
// so a page rendered from a copy of the logs is byte-identical to one
// rendered in the project.
package runview
