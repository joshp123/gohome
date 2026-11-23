package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Canonical exit codes shared across gohome-cli and gohome-act.
const (
	ExitOK             = 0
	ExitRuntimeError   = 1
	ExitUsageError     = 2
	ExitPolicyDenied   = 3
	ExitApprovalDenied = 4
)

// OutputFormat selects human-readable tabular output or structured JSON.
type OutputFormat int

const (
	FormatText OutputFormat = iota
	FormatJSON
)

// KeyValue represents one field in a detail view.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// HelpSection groups related commands in CLI help output.
type HelpSection struct {
	Heading string
	Lines   []string
}

// ErrorEnvelope is the structured JSON shape emitted for errors when --json is set.
type ErrorEnvelope struct {
	Action string `json:"action"`
	Error  string `json:"error"`
	Code   int    `json:"code"`
}

// Renderer is the single output path for gohome-cli and gohome-act.
type Renderer struct {
	Out    io.Writer
	Err    io.Writer
	Format OutputFormat
}

// NewRenderer builds a Renderer writing to out and errOut.
func NewRenderer(out, errOut io.Writer, format OutputFormat) Renderer {
	return Renderer{Out: out, Err: errOut, Format: format}
}

// WriteTable writes a compact aligned table with uppercase headers.
func (r Renderer) WriteTable(headers []string, rows [][]string) error {
	w := tabwriter.NewWriter(r.Out, 2, 4, 2, ' ', 0)
	if len(headers) > 0 {
		if _, err := fmt.Fprintln(w, strings.Join(headers, "\t")); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(w, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return w.Flush()
}

// WriteKeyValues writes a compact two-column key/value table.
func (r Renderer) WriteKeyValues(pairs []KeyValue) error {
	rows := make([][]string, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, []string{p.Key, p.Value})
	}
	return r.WriteTable([]string{"FIELD", "VALUE"}, rows)
}

// WriteProto writes a protobuf message formatted with canonical protojson.
func (r Renderer) WriteProto(msg proto.Message) error {
	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.Out, "%s\n", data)
	return err
}

// WriteProtoCompact writes a single-line protojson record (used for streaming events).
func (r Renderer) WriteProtoCompact(msg proto.Message) error {
	data, err := protojson.MarshalOptions{}.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.Out, "%s\n", data)
	return err
}

// WriteJSON writes an indented JSON representation of a typed value.
func (r Renderer) WriteJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.Out, "%s\n", data)
	return err
}

// WriteLine writes one plain line followed by a newline.
func (r Renderer) WriteLine(text string) error {
	_, err := fmt.Fprintln(r.Out, text)
	return err
}

// WriteRaw writes raw preformatted text (such as OpenMetrics exposition).
func (r Renderer) WriteRaw(text string) error {
	_, err := io.WriteString(r.Out, text)
	return err
}

// WriteError emits an error consistently in text or JSON format.
func (r Renderer) WriteError(action string, err error, exitCode int) {
	if r.Format == FormatJSON {
		env := ErrorEnvelope{Action: action, Error: err.Error(), Code: exitCode}
		if data, marshalErr := json.MarshalIndent(env, "", "  "); marshalErr == nil {
			_, _ = fmt.Fprintf(r.Err, "%s\n", data)
			return
		}
	}
	_, _ = fmt.Fprintf(r.Err, "%s: %v\n", action, err)
}

// WriteHelp renders CLI usage consistently across all subcommands.
func (r Renderer) WriteHelp(usage string, sections []HelpSection) {
	_, _ = fmt.Fprintln(r.Out, usage)
	for _, sec := range sections {
		_, _ = fmt.Fprintln(r.Out, "")
		if sec.Heading != "" {
			_, _ = fmt.Fprintf(r.Out, "%s:\n", sec.Heading)
		}
		for _, line := range sec.Lines {
			_, _ = fmt.Fprintf(r.Out, "  %s\n", line)
		}
	}
}
