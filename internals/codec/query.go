package codec

import (
	"fmt"
	"strings"
)

type QueryParam struct {
	Key   string
	Value string
	HasEq bool
}

type Query struct {
	Params    []QueryParam
	HasPrefix bool
}

func parseQuery(raw string) (*Query, error) {
	if raw == "" {
		return &Query{}, nil
	}

	query := &Query{
		HasPrefix: true,
	}

	parts := strings.Split(raw, "&")

	for _, part := range parts {
		if part == "" {
			query.Params = append(query.Params, QueryParam{})
			continue
		}

		key, value, hasEq := strings.Cut(part, "=")

		query.Params = append(query.Params, QueryParam{
			Key:   key,
			Value: value,
			HasEq: hasEq,
		})
	}

	return query, nil
}

func (q *Query) String() string {
	if !q.HasPrefix || len(q.Params) == 0 {
		return ""
	}

	var builder strings.Builder

	for i, param := range q.Params {
		if i > 0 {
			builder.WriteByte('&')
		}

		builder.WriteString(param.Key)

		if param.HasEq {
			builder.WriteByte('=')
			builder.WriteString(param.Value)
		}
	}

	return builder.String()
}

func validateQuery(q *Query) error {
	if q == nil {
		return fmt.Errorf("query is nil")
	}

	return nil
}
