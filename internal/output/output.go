package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

type Printer struct {
	JSON   bool
	Stdout io.Writer
	Stderr io.Writer
}

func New(jsonMode bool, stdout, stderr io.Writer) *Printer {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	return &Printer{JSON: jsonMode, Stdout: stdout, Stderr: stderr}
}

func (p *Printer) Result(v any) error {
	if p.JSON {
		return p.writeJSON(p.Stdout, v)
	}
	if s, ok := v.(string); ok {
		_, err := fmt.Fprintln(p.Stdout, s)
		return err
	}
	return p.writeJSON(p.Stdout, v)
}

func (p *Printer) Success(extra map[string]any) error {
	payload := map[string]any{"success": true}
	for k, v := range extra {
		payload[k] = v
	}
	if p.JSON {
		return p.writeJSON(p.Stdout, payload)
	}
	if msg, ok := extra["message"].(string); ok && msg != "" {
		_, err := fmt.Fprintln(p.Stdout, msg)
		return err
	}
	_, err := fmt.Fprintln(p.Stdout, "ok")
	return err
}

func (p *Printer) Table(headers []string, rows [][]string) error {
	if p.JSON {
		return p.writeJSON(p.Stdout, map[string]any{
			"success": true,
			"headers": headers,
			"rows":    rows,
		})
	}
	w := tabwriter.NewWriter(p.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

func (p *Printer) PrintError(err error) {
	if err == nil {
		return
	}
	ae, ok := apperr.As(err)
	if p.JSON {
		payload := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":      "INTERNAL",
				"message":   err.Error(),
				"retryable": false,
			},
		}
		if ok {
			payload["error"] = map[string]any{
				"code":      ae.Code,
				"message":   ae.Message,
				"retryable": ae.Retryable,
				"details":   ae.Details,
			}
		}
		_ = p.writeJSON(p.Stdout, payload)
		return
	}
	fmt.Fprintln(p.Stderr, "error:", err.Error())
}

func (p *Printer) writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
