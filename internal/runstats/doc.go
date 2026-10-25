// Package runstats is the read side of internal/tracelog: it turns a logs
// directory into workflow runs and reports where each run's time went.
//
// I/O sits at the package's two edges. Load reads a logs directory,
// read-only, one line at a time; the renderers write plain text tables to an
// io.Writer. Everything between them is a pure function from records to
// results, with no clock and no configuration, so the figures are the same
// for a project's own logs and for a copy of them taken anywhere.
//
// The package never creates, writes, renames or deletes anything in the
// logs directory.
package runstats
