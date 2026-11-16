package gosmos

import (
	"reflect"
	"strings"
)

// SelectAs selects the top-level JSON fields of a result struct.
func SelectAs[R any, T Entity](o *Orbit[T]) *Orbit[T] {
	typ := reflect.TypeFor[R]()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		o.err = validation("projection result must be a struct")
		return o
	}
	var names []string
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if f.Anonymous && tag == "" && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			if tag == "" {
				tag = f.Name
			}
			names = append(names, tag)
		}
	}
	walk(typ)
	if len(names) == 0 {
		o.err = validation("projection result has no JSON fields")
		return o
	}
	return o.Select(names...)
}
