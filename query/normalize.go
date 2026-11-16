package query

import (
	"strconv"
	"strings"
	"unicode"
)

// Normalize replaces parameters outside quoted literals and property names with
// @p0, @p1, ... in first-occurrence order, returning their original unique names.
func Normalize(sql string) (string, []string) {
	var b strings.Builder
	var names []string
	indices := map[string]int{}
	var quote byte
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(sql) {
				i++
				b.WriteByte(sql[i])
			} else if c == quote {
				if i+1 < len(sql) && sql[i+1] == quote {
					i++
					b.WriteByte(sql[i])
				} else {
					quote = 0
				}
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
		} else if c == '@' {
			end := i + 1
			for end < len(sql) && (unicode.IsLetter(rune(sql[end])) || sql[end] >= '0' && sql[end] <= '9' || sql[end] == '_') {
				end++
			}
			if end > i+1 {
				name := sql[i:end]
				index, ok := indices[name]
				if !ok {
					index = len(names)
					indices[name] = index
					names = append(names, name)
				}
				b.WriteString("@p" + strconv.Itoa(index))
				i = end - 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String(), names
}
