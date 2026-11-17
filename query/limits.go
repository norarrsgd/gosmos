package query

import "fmt"

// Query budgets bound synchronous generation work, independently of Cosmos limits.
const (
	MaxQueryNodes          = 1024
	MaxQueryTextBytes      = 64 * 1024
	MaxQueryParameters     = 1024
	MaxSQLBytes            = 256 * 1024
	MaxProjectionDepth     = 32
	MaxProjectionJSONBytes = 256 * 1024
	MaxFullTextTerms       = 64
	MaxFullTextTermBytes   = 1024
	MaxFullTextBytes       = 64 * 1024
)

type queryBudget struct {
	nodes, textBytes, parameters, termBytes int
}

func (b *queryBudget) take(nodes int, text ...string) error {
	if nodes > MaxQueryNodes-b.nodes {
		return fmt.Errorf("query exceeds %d structural nodes", MaxQueryNodes)
	}
	b.nodes += nodes
	for _, s := range text {
		if len(s) > MaxQueryTextBytes-b.textBytes {
			return fmt.Errorf("query exceeds %d metadata bytes", MaxQueryTextBytes)
		}
		b.textBytes += len(s)
	}
	return nil
}

func (b *queryBudget) projection(p *Projection, depth int) error {
	if depth > MaxProjectionDepth {
		return fmt.Errorf("projection exceeds depth %d", MaxProjectionDepth)
	}
	if err := b.take(len(p.items)); err != nil {
		return err
	}
	for _, item := range p.items {
		if err := b.take(0, item.name, item.column, item.alias, string(item.aggregate)); err != nil {
			return err
		}
		if item.object != nil {
			if err := b.projection(item.object, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateInputBudget runs before rendering, hashing identities, or expanding terms.
func validateInputBudget(in Input) error {
	b := queryBudget{}
	if err := b.take(len(in.Clusters) + len(in.Groups) + len(in.Sorting)); err != nil {
		return err
	}
	for _, group := range in.Groups {
		if err := b.take(0, group); err != nil {
			return err
		}
	}
	for _, sort := range in.Sorting {
		if err := b.take(0, sort.Column, string(sort.Direction)); err != nil {
			return err
		}
	}
	columns := func(names []string, aggregates []AggregationOption) error {
		if err := b.take(len(names) + len(aggregates)); err != nil {
			return err
		}
		for _, name := range names {
			if err := b.take(0, name); err != nil {
				return err
			}
		}
		for _, ag := range aggregates {
			if err := b.take(0, ag.Column, string(ag.Aggregate)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := columns(in.Columns.Names, in.Columns.Aggregates); err != nil {
		return err
	}
	if j := in.Columns.Join; j != nil {
		if err := b.take(1, j.Alias, j.ArrayPath); err != nil {
			return err
		}
		if err := columns(j.Columns, j.Aggregates); err != nil {
			return err
		}
	}
	if p := in.Columns.Projection; p != nil {
		if err := b.projection(p, 1); err != nil {
			return err
		}
	}
	for _, cluster := range in.Clusters {
		if err := b.take(len(cluster.Catalysts), string(cluster.Where)); err != nil {
			return err
		}
		for _, c := range cluster.Catalysts {
			if err := b.take(0, c.Column, c.Alias, c.ID, string(c.Operator), string(c.Where)); err != nil {
				return err
			}
			for _, operand := range []*Operand{c.Left, c.Right} {
				if operand != nil {
					if err := b.take(1, operand.Path, operand.Alias, string(operand.Kind)); err != nil {
						return err
					}
				}
			}
			parameters := 0
			if !c.unary() && (c.Right == nil || c.Right.Kind == ParameterOperand) {
				parameters = 1
			}
			switch c.op() {
			case FTScore, FTContainsAll, NotFTContainsAll, FTContainsAny, NotFTContainsAny:
				if terms, ok := c.Value.([]string); ok {
					if len(terms) > MaxFullTextTerms {
						return fmt.Errorf("full-text operation exceeds %d terms", MaxFullTextTerms)
					}
					for _, term := range terms {
						if len(term) > MaxFullTextTermBytes {
							return fmt.Errorf("full-text term exceeds %d bytes", MaxFullTextTermBytes)
						}
						if len(term) > MaxFullTextBytes-b.termBytes {
							return fmt.Errorf("query exceeds %d full-text bytes", MaxFullTextBytes)
						}
						b.termBytes += len(term)
					}
					if c.op() == FTScore {
						parameters = len(terms)
					}
				}
			}
			if parameters > MaxQueryParameters-b.parameters {
				return fmt.Errorf("query exceeds %d parameters", MaxQueryParameters)
			}
			b.parameters += parameters
		}
	}
	return nil
}
