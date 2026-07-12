package domain

import "fmt"

// Severity is stable machine-readable diagnostic severity.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// SourceLocation identifies where a diagnostic originated.
type SourceLocation struct {
	File     string `json:"file,omitempty"`
	Document int    `json:"document,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

// Diagnostic is the common compiler diagnostic model.
type Diagnostic struct {
	Severity    Severity       `json:"severity"`
	Code        string         `json:"code"`
	Resource    string         `json:"resource,omitempty"`
	FieldPath   string         `json:"fieldPath,omitempty"`
	Message     string         `json:"message"`
	Remediation string         `json:"remediation,omitempty"`
	Location    SourceLocation `json:"location,omitempty"`
}

func (d Diagnostic) Error() string {
	if d.Resource == "" {
		return fmt.Sprintf("%s %s: %s", d.Severity, d.Code, d.Message)
	}
	return fmt.Sprintf("%s %s %s: %s", d.Severity, d.Code, d.Resource, d.Message)
}

func HasErrors(diags []Diagnostic) bool {
	for _, diag := range diags {
		if diag.Severity == SeverityError {
			return true
		}
	}
	return false
}
