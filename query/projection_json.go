package query

import "encoding/json"

type projectionJSON struct {
	Items []projectionItemJSON `json:"items"`
}
type projectionItemJSON struct {
	Name      string      `json:"name"`
	Column    string      `json:"column,omitempty"`
	Alias     string      `json:"alias,omitempty"`
	Aggregate Aggregate   `json:"aggregate,omitempty"`
	Object    *Projection `json:"object,omitempty"`
}

// MarshalJSON encodes the language-neutral structured projection tree.
func (p Projection) MarshalJSON() ([]byte, error) {
	wire := projectionJSON{Items: make([]projectionItemJSON, len(p.items))}
	for i, item := range p.items {
		wire.Items[i] = projectionItemJSON{Name: item.name, Column: item.column, Alias: item.alias, Aggregate: item.aggregate, Object: item.object}
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes a structured projection; Build validates its names and paths.
func (p *Projection) UnmarshalJSON(data []byte) error {
	var wire projectionJSON
	if e := json.Unmarshal(data, &wire); e != nil {
		return e
	}
	items := make([]projectionItem, len(wire.Items))
	for i, item := range wire.Items {
		alias := item.Alias
		if alias == "" {
			alias = "c"
		}
		items[i] = projectionItem{name: item.Name, column: item.Column, alias: alias, aggregate: item.Aggregate, object: item.Object}
	}
	p.items = items
	return nil
}
