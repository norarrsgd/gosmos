package repository

import (
	"context"
	"encoding/json"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"reflect"
	"strconv"
	"strings"
)

// Patch builds at most ten operations using JSON property paths of T.
// It is a mutable request builder and must not be shared between goroutines.
type Patch[T Entity] struct {
	operations azcosmos.PatchOperations
	count      int
	err        error
}

func (p *Patch[T]) append(path string, apply func(string)) *Patch[T] {
	if p.err != nil {
		return p
	}
	if p.count == 10 {
		p.err = validation("patch supports at most 10 operations")
		return p
	}
	normalized, err := patchPath(reflect.TypeFor[T](), path)
	if err != nil {
		p.err = err
		return p
	}
	apply(normalized)
	p.count++
	return p
}
func (p *Patch[T]) Set(path string, value any) *Patch[T] {
	return p.append(path, func(path string) { p.operations.AppendSet(path, patchValue(value)) })
}
func (p *Patch[T]) Add(path string, value any) *Patch[T] {
	return p.append(path, func(path string) { p.operations.AppendAdd(path, patchValue(value)) })
}
func (p *Patch[T]) Replace(path string, value any) *Patch[T] {
	return p.append(path, func(path string) { p.operations.AppendReplace(path, patchValue(value)) })
}
func (p *Patch[T]) Remove(path string) *Patch[T] {
	return p.append(path, func(path string) { p.operations.AppendRemove(path) })
}
func (p *Patch[T]) Increment(path string, value int64) *Patch[T] {
	return p.append(path, func(path string) { p.operations.AppendIncrement(path, value) })
}
func (p *Patch[T]) Build() (azcosmos.PatchOperations, error) {
	if p.err != nil {
		return azcosmos.PatchOperations{}, p.err
	}
	if p.count == 0 {
		return azcosmos.PatchOperations{}, validation("patch requires an operation")
	}
	if _, err := json.Marshal(p.operations); err != nil {
		return azcosmos.PatchOperations{}, validation("invalid patch value: %v", err)
	}
	return p.operations, nil
}

func patchValue(value any) any {
	// The SDK omits a nil interface instead of serializing the required null value.
	if value == nil {
		return json.RawMessage("null")
	}
	return value
}
func patchPath(t reflect.Type, path string) (string, error) {
	if path == "" {
		return "", validation("patch path is empty")
	}
	var parts []string
	if strings.HasPrefix(path, "/") {
		parts = strings.Split(path[1:], "/")
		for i, s := range parts {
			for j := 0; j < len(s); j++ {
				if s[j] == '~' {
					if j+1 >= len(s) || (s[j+1] != '0' && s[j+1] != '1') {
						return "", validation("invalid JSON pointer escape")
					}
					j++
				}
			}
			parts[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
		}
	} else {
		parts = strings.Split(path, ".")
	}
	if parts[0] == "id" || strings.HasPrefix(parts[0], "_") {
		return "", validation("patch cannot modify identity or system properties")
	}
	for i, part := range parts {
		if part == "" {
			return "", validation("empty patch path component")
		}
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			field, ok := jsonField(t, part)
			if !ok {
				return "", validation("unknown JSON property %q", part)
			}
			t = field.Type
		case reflect.Map:
			if t.Key().Kind() != reflect.String {
				return "", validation("patch map keys must be strings")
			}
			t = t.Elem()
		case reflect.Array, reflect.Slice:
			if part == "-" && i != len(parts)-1 {
				return "", validation("array append marker must be the final path component")
			}
			if part != "-" {
				n, err := strconv.Atoi(part)
				if err != nil || n < 0 || strconv.Itoa(n) != part {
					return "", validation("invalid array index %q", part)
				}
			}
			t = t.Elem()
		default:
			return "", validation("patch path traverses a scalar")
		}
	}
	escaped := make([]string, len(parts))
	for i, part := range parts {
		escaped[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
	}
	return "/" + strings.Join(escaped, "/"), nil
}
func jsonField(t reflect.Type, name string) (reflect.StructField, bool) {
	return findJSONField(t, name, make(map[reflect.Type]bool))
}
func findJSONField(t reflect.Type, name string, visited map[reflect.Type]bool) (reflect.StructField, bool) {
	if visited[t] {
		return reflect.StructField{}, false
	}
	visited[t] = true
	defer delete(visited, t)
	for i := range t.NumField() {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == name || (tag == "" && f.Name == name) {
			return f, true
		}
		if f.Anonymous && tag == "" {
			embedded := f.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				if found, ok := findJSONField(embedded, name, visited); ok {
					return found, true
				}
			}
		}
	}
	return reflect.StructField{}, false
}

type patchContainer interface {
	PatchItem(context.Context, azcosmos.PartitionKey, string, azcosmos.PatchOperations, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
}

// Patch updates one document with validated paths and optional ETag protection.
func (g *Galaxy[T]) Patch(ctx context.Context, id string, pk []string, build func(*Patch[T]), options ...WriteOption) (Gravity, error) {
	if err := ctx.Err(); err != nil {
		return Gravity{}, g.failure("patch", err)
	}
	if strings.TrimSpace(id) == "" || build == nil {
		return Gravity{}, g.failure("patch", validation("document ID and patch callback are required"))
	}
	key, err := stringKey(pk, len(g.metadata.fields))
	if err != nil {
		return Gravity{}, g.failure("patch", err)
	}
	p := &Patch[T]{}
	build(p)
	operations, err := p.Build()
	if err != nil {
		return Gravity{}, g.failure("patch", err)
	}
	o, err := writeOptions(options)
	if err != nil {
		return Gravity{}, g.failure("patch", err)
	}
	container, ok := g.container.(patchContainer)
	if !ok {
		return Gravity{}, g.failure("patch", validation("container does not support patch"))
	}
	response, err := container.PatchItem(ctx, key, id, operations, o)
	if g.options.cache != nil {
		g.options.cache.Clear()
	}
	return gravity(response), g.failure("patch", err)
}
