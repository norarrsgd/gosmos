package gosmos

import (
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type keyField struct {
	index []int
	path  string
}
type entityMetadata struct {
	fields   []keyField
	override bool
	name     string
}
type metadataResult struct {
	metadata entityMetadata
	err      error
}

var entityTypes sync.Map

func inspectEntity[T Entity]() (entityMetadata, error) {
	t := reflect.TypeFor[T]()
	if r, ok := entityTypes.Load(t); ok {
		result := r.(metadataResult)
		return result.metadata, result.err
	}
	m, err := inspectType(t)
	entityTypes.Store(t, metadataResult{m, err})
	return m, err
}
func inspectType(t reflect.Type) (entityMetadata, error) {
	m := entityMetadata{name: t.String()}
	base := t
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return m, validation("entity must be a struct or pointer to struct")
	}
	for i := range base.NumField() {
		f := base.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Pointer {
			return m, validation("embed entity capabilities by value, not pointer")
		}
	}
	contract := reflect.TypeFor[PartitionKeyer]()
	m.override = t.Implements(contract) || reflect.PointerTo(base).Implements(contract)
	numbered := map[int]keyField{}
	var walk func(reflect.Type, []int, string) error
	walk = func(current reflect.Type, parent []int, path string) error {
		for i := range current.NumField() {
			f := current.Field(i)
			tag := f.Tag.Get("gosmos")
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				if tag != "" {
					return validation("partition key field %s is excluded from JSON", f.Name)
				}
				continue
			}
			if f.PkgPath != "" {
				if tag != "" {
					return validation("partition key field %s is unexported", f.Name)
				}
				continue
			}
			index := append(append([]int(nil), parent...), i)
			if name == "" {
				name = f.Name
			}
			fieldPath := path + "/" + name
			if tag != "" {
				if !strings.HasPrefix(tag, "pk=") {
					return validation("invalid gosmos tag %q", tag)
				}
				n, err := strconv.Atoi(strings.TrimPrefix(tag, "pk="))
				if err != nil || n < 1 || n > 3 {
					return validation("partition key levels must be 1 to 3")
				}
				if _, ok := numbered[n]; ok {
					return validation("duplicate partition key level %d", n)
				}
				kind := f.Type.Kind()
				if kind == reflect.Pointer {
					kind = f.Type.Elem().Kind()
				}
				if kind != reflect.String {
					return validation("partition key field %s must be a string or *string", f.Name)
				}
				numbered[n] = keyField{index, fieldPath}
				continue
			}
			if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
				if err := walk(f.Type, index, path); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if m.override {
		return m, nil
	}
	if err := walk(base, nil, ""); err != nil {
		return m, err
	}
	if len(numbered) == 0 {
		return m, validation("entity requires partition key tags or PartitionKeyer")
	}
	for n := 1; n <= len(numbered); n++ {
		f, ok := numbered[n]
		if !ok {
			return m, validation("partition key levels must not have gaps")
		}
		m.fields = append(m.fields, f)
	}
	return m, nil
}
func (m entityMetadata) key(value reflect.Value) (azcosmos.PartitionKey, error) {
	if m.override {
		p, ok := value.Interface().(PartitionKeyer)
		if !ok && value.CanAddr() {
			p, ok = value.Addr().Interface().(PartitionKeyer)
		}
		if !ok {
			return azcosmos.PartitionKey{}, validation("PartitionKeyer unavailable")
		}
		return stringKey(p.PartitionKeyValues(), 0)
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	key := azcosmos.NewPartitionKey()
	for _, field := range m.fields {
		v := value.FieldByIndex(field.index)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return key, validation("partition key %s is null", field.path)
			}
			v = v.Elem()
		}
		key = key.AppendString(v.String())
	}
	return key, nil
}
func stringKey(values []string, expected int) (azcosmos.PartitionKey, error) {
	key := azcosmos.NewPartitionKey()
	if len(values) < 1 || len(values) > 3 || (expected > 0 && len(values) != expected) {
		return key, validation("expected %d partition key components, received %d", expected, len(values))
	}
	for _, v := range values {
		key = key.AppendString(v)
	}
	return key, nil
}
