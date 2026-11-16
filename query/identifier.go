package query

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

var reserved = map[string]bool{"AND": true, "AS": true, "ASC": true, "BETWEEN": true, "BY": true, "DESC": true, "DISTINCT": true, "FROM": true, "GROUP": true, "IN": true, "JOIN": true, "NOT": true, "OR": true, "ORDER": true, "RANK": true, "SELECT": true, "TOP": true, "WHERE": true}

// ValidateIdentifier checks a source property path before bracket formatting.
func ValidateIdentifier(id string) error {
	if strings.TrimSpace(strings.ReplaceAll(id, ".", "")) == "" {
		return fmt.Errorf("identifier must contain a property name")
	}
	if utf8.RuneCountInString(id) > 255 || !utf8.ValidString(id) {
		return fmt.Errorf("invalid identifier length or encoding")
	}
	if strings.ContainsAny(id, ";\"]") {
		return fmt.Errorf("identifier contains an SQL delimiter")
	}
	if strings.Contains(id, "--") || strings.Contains(id, "/*") || strings.Contains(id, "*/") {
		return fmt.Errorf("identifier contains an SQL comment")
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return fmt.Errorf("identifier contains a control character")
		}
	}
	return nil
}
func validateAlias(alias string) error {
	if len(alias) == 0 || len(alias) > 128 || reserved[strings.ToUpper(alias)] {
		return fmt.Errorf("invalid SQL alias %q", alias)
	}
	for i, r := range alias {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_' || i > 0 && r >= '0' && r <= '9') {
			return fmt.Errorf("invalid SQL alias %q", alias)
		}
	}
	return nil
}
func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 32 || r > 126 || strings.ContainsRune("\"'&+<>`", r) {
				for _, unit := range utf16.Encode([]rune{r}) {
					fmt.Fprintf(&b, `\u%04X`, unit)
				}
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// FormatProperty validates and bracket-formats a source path without a naming policy.
func FormatProperty(alias, path string) (string, error) { return formatProperty(alias, path, nil) }
func formatProperty(alias, path string, policy func(string) string) (string, error) {
	if alias == "" {
		alias = "c"
	}
	if e := validateAlias(alias); e != nil {
		return "", e
	}
	if e := ValidateIdentifier(path); e != nil {
		return "", e
	}
	var b strings.Builder
	b.WriteString(alias)
	for _, part := range strings.Split(path, ".") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		if policy != nil {
			part = policy(part)
		}
		if e := ValidateIdentifier(part); e != nil {
			return "", fmt.Errorf("naming policy: %w", e)
		}
		b.WriteByte('[')
		b.WriteString(jsonString(part))
		b.WriteByte(']')
	}
	return b.String(), nil
}
func aliasBase(path string, policy func(string) string) string {
	var parts []string
	for _, p := range strings.Split(path, ".") {
		p = strings.TrimSpace(p)
		if p != "" {
			if policy != nil {
				p = policy(p)
			}
			parts = append(parts, p)
		}
	}
	var b strings.Builder
	underscore := false
	for _, r := range strings.Join(parts, "_") {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			r = '_'
		}
		if r == '_' && underscore {
			continue
		}
		b.WriteRune(r)
		underscore = r == '_'
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		s = "value"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	if reserved[strings.ToUpper(s)] {
		s += "_alias"
	}
	return s
}
