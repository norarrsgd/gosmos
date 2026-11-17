package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type projectionJSON struct {
	Items []projectionItemJSON `json:"items"`
}
type projectionItemJSON struct {
	Name      string          `json:"name"`
	Column    string          `json:"column,omitempty"`
	Alias     string          `json:"alias,omitempty"`
	Aggregate Aggregate       `json:"aggregate,omitempty"`
	Object    *projectionJSON `json:"object,omitempty"`
}

// MarshalJSON encodes the language-neutral structured projection tree.
func (p Projection) MarshalJSON() ([]byte, error) {
	budget := queryBudget{}
	if err := budget.projection(&p, 1); err != nil {
		return nil, err
	}
	data, err := json.Marshal(projectionWire(p))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxProjectionJSONBytes {
		return nil, fmt.Errorf("projection exceeds %d JSON bytes", MaxProjectionJSONBytes)
	}
	return data, nil
}

func projectionWire(p Projection) projectionJSON {
	wire := projectionJSON{Items: make([]projectionItemJSON, len(p.items))}
	for i, item := range p.items {
		wire.Items[i] = projectionItemJSON{Name: item.name, Column: item.column, Alias: item.alias, Aggregate: item.aggregate}
		if item.object != nil {
			child := projectionWire(*item.object)
			wire.Items[i].Object = &child
		}
	}
	return wire
}

// UnmarshalJSON decodes a structured projection; Build validates its names and paths.
func (p *Projection) UnmarshalJSON(data []byte) error {
	if len(data) > MaxProjectionJSONBytes {
		return fmt.Errorf("projection exceeds %d JSON bytes", MaxProjectionJSONBytes)
	}
	// Bound even unknown JSON fields before the standard decoder builds the wire tree.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{', '[':
				depth++
				if depth > 3*MaxProjectionDepth {
					return fmt.Errorf("projection JSON exceeds nesting budget")
				}
			case '}', ']':
				depth--
			}
		}
	}
	var wire projectionJSON
	if e := json.Unmarshal(data, &wire); e != nil {
		return e
	}
	budget := queryBudget{}
	decoded, err := wire.projection(1, &budget)
	if err != nil {
		return err
	}
	*p = decoded
	return nil
}

func (wire projectionJSON) projection(depth int, budget *queryBudget) (Projection, error) {
	if depth > MaxProjectionDepth {
		return Projection{}, fmt.Errorf("projection exceeds depth %d", MaxProjectionDepth)
	}
	if err := budget.take(len(wire.Items)); err != nil {
		return Projection{}, err
	}
	items := make([]projectionItem, len(wire.Items))
	for i, item := range wire.Items {
		alias := item.Alias
		if alias == "" {
			alias = "c"
		}
		if err := budget.take(0, item.Name, item.Column, alias, string(item.Aggregate)); err != nil {
			return Projection{}, err
		}
		items[i] = projectionItem{name: item.Name, column: item.Column, alias: alias, aggregate: item.Aggregate}
		if item.Object != nil {
			child, err := item.Object.projection(depth+1, budget)
			if err != nil {
				return Projection{}, err
			}
			items[i].object = &child
		}
	}
	return Projection{items: items}, nil
}
