package domain

import (
	"fmt"
	"strings"
)

// ResourceIdentity is the stable logical identity of a generated or source resource.
type ResourceIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Target     string `json:"target,omitempty"`
	Adapter    string `json:"adapter,omitempty"`
}

func (id ResourceIdentity) CanonicalString() string {
	parts := []string{
		"apiVersion=" + escapeIdentityPart(id.APIVersion),
		"kind=" + escapeIdentityPart(id.Kind),
		"namespace=" + escapeIdentityPart(id.Namespace),
		"name=" + escapeIdentityPart(id.Name),
	}
	if id.Target != "" {
		parts = append(parts, "target="+escapeIdentityPart(id.Target))
	}
	if id.Adapter != "" {
		parts = append(parts, "adapter="+escapeIdentityPart(id.Adapter))
	}
	return strings.Join(parts, ";")
}

func (id ResourceIdentity) Display() string {
	ns := id.Namespace
	if ns == "" {
		ns = "default"
	}
	if id.Target == "" && id.Adapter == "" {
		return fmt.Sprintf("%s/%s/%s", id.Kind, ns, id.Name)
	}
	return fmt.Sprintf("%s/%s/%s target=%s adapter=%s", id.Kind, ns, id.Name, id.Target, id.Adapter)
}

func escapeIdentityPart(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `;`, `\;`)
	value = strings.ReplaceAll(value, `=`, `\=`)
	return value
}
