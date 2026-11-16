package query

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ranked struct {
	c     Catalyst
	name  string
	index int
}

func (b *builder) rankExpr(r ranked) (string, error) {
	p, e := b.property(r.c.Alias, r.c.Column)
	if e != nil {
		return "", e
	}
	if r.c.op() == VectorDistance {
		return "VectorDistance(" + p + ", @" + r.name + ")", nil
	}
	var params []string
	for i := range r.c.Value.([]string) {
		params = append(params, "@"+r.name+"_ft"+strconv.Itoa(i))
	}
	return "FullTextScore(" + p + ", " + strings.Join(params, ", ") + ")", nil
}

func (b *builder) order() (string, error) {
	var fields []string
	var weights []string
	for _, s := range b.input.Sorting {
		if s.Direction == Weighted {
			w, e := validateWeights(s.Column)
			if e != nil {
				return "", e
			}
			weights = append(weights, w)
			continue
		}
		direction := s.Direction
		if direction == "" {
			direction = Asc
		}
		if direction != Asc && direction != Desc {
			return "", fmt.Errorf("invalid sorting direction")
		}
		p, e := b.property("c", s.Column)
		if e != nil {
			return "", e
		}
		fields = append(fields, p+" "+string(direction))
	}
	if len(b.ranks) == 0 {
		if len(weights) > 0 {
			return "", fmt.Errorf("weights require multiple rank predicates")
		}
		if len(fields) == 0 {
			return "", nil
		}
		return " ORDER BY " + strings.Join(fields, ", "), nil
	}
	if len(fields) > 0 {
		return "", fmt.Errorf("scalar sorting cannot accompany ranking")
	}
	if len(weights) > 1 {
		return "", fmt.Errorf("only one weight option is allowed")
	}
	var ranks []string
	for _, r := range b.ranks {
		expr, e := b.rankExpr(r)
		if e != nil {
			return "", e
		}
		ranks = append(ranks, expr)
	}
	if len(ranks) > 1 {
		b.shape.RRF = true
		if len(weights) > 0 {
			ranks = append(ranks, weights[0])
		}
		return " ORDER BY RANK RRF(" + strings.Join(ranks, ", ") + ")", nil
	}
	if len(weights) > 0 {
		return "", fmt.Errorf("weights require multiple rank predicates")
	}
	prefix := " ORDER BY "
	if b.ranks[0].c.op() == FTScore {
		prefix += "RANK "
	}
	return prefix + ranks[0], nil
}

func validateWeights(value string) (string, error) {
	inner := strings.TrimSpace(value)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	if strings.TrimSpace(inner) == "" {
		return "", fmt.Errorf("weights cannot be empty")
	}
	for _, r := range inner {
		if !(r >= '0' && r <= '9' || r == '.' || r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r') {
			return "", fmt.Errorf("invalid weight character")
		}
	}
	for _, part := range strings.Split(inner, ",") {
		v, e := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return "", fmt.Errorf("invalid weight")
		}
	}
	return "[" + inner + "]", nil
}
