package codec

import (
	"fmt"
	"strings"
)

type QueryKeyToken struct {
	Value string
	Code  uint64
	Bits  uint8
}

var queryKeyTokens = []QueryKeyToken{
	{Value: "id", Code: 0b00000, Bits: 5},
	{Value: "page", Code: 0b00001, Bits: 5},
	{Value: "q", Code: 0b00010, Bits: 5},
	{Value: "query", Code: 0b00011, Bits: 5},
	{Value: "search", Code: 0b00100, Bits: 5},
	{Value: "sort", Code: 0b00101, Bits: 5},
	{Value: "filter", Code: 0b00110, Bits: 5},
	{Value: "limit", Code: 0b00111, Bits: 5},
	{Value: "offset", Code: 0b01000, Bits: 5},
	{Value: "lang", Code: 0b01001, Bits: 5},
	{Value: "type", Code: 0b01010, Bits: 5},
	{Value: "token", Code: 0b01011, Bits: 5},
	{Value: "redirect", Code: 0b01100, Bits: 5},
	{Value: "utm_source", Code: 0b01101, Bits: 5},
	{Value: "utm_medium", Code: 0b01110, Bits: 5},
	{Value: "utm_campaign", Code: 0b01111, Bits: 5},
}

var (
	queryKeyEncode map[string]QueryKeyToken
	queryKeyDecode map[uint64]string
)

func init() {
	queryKeyEncode = make(map[string]QueryKeyToken)
	queryKeyDecode = make(map[uint64]string)

	for _, token := range queryKeyTokens {
		queryKeyEncode[token.Value] = token

		key := (uint64(token.Bits) << 56) | token.Code
		queryKeyDecode[key] = token.Value
	}
}

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

func encodeQueryKey(w *BitWriter, key string) error {
	if token, ok := queryKeyEncode[key]; ok {
		if err := w.WriteBits(0, 1); err != nil {
			return err
		}

		return w.WriteBits(token.Code, token.Bits)
	}

	if err := w.WriteBits(1, 1); err != nil {
		return err
	}

	if len(key) > 63 {
		return fmt.Errorf(
			"query key too long: %q",
			key,
		)
	}

	if err := w.WriteBits(uint64(len(key)), 6); err != nil {
		return err
	}

	for i := 0; i < len(key); i++ {
		if err := w.WriteBits(uint64(key[i]), 8); err != nil {
			return err
		}
	}

	return nil
}

func decodeQueryKey(r *BitReader) (string, error) {
	kind, err := r.ReadBits(1)
	if err != nil {
		return "", err
	}

	if kind == 1 {
		length, err := r.ReadBits(6)
		if err != nil {
			return "", err
		}

		data := make([]byte, length)

		for i := range data {
			value, err := r.ReadBits(8)
			if err != nil {
				return "", err
			}

			data[i] = byte(value)
		}

		return string(data), nil
	}

	code, err := r.ReadBits(5)
	if err != nil {
		return "", err
	}

	key := (uint64(5) << 56) | code

	value, ok := queryKeyDecode[key]
	if !ok {
		return "", fmt.Errorf(
			"unknown query key token: %05b",
			code,
		)
	}

	return value, nil
}

func encodeQueryParam(w *BitWriter, param QueryParam) error {
	if err := encodeQueryKey(w, param.Key); err != nil {
		return err
	}

	if param.HasEq {
		if err := w.WriteBits(1, 1); err != nil {
			return err
		}

		if len(param.Value) > 255 {
			return fmt.Errorf(
				"query value too long: %d bytes",
				len(param.Value),
			)
		}

		if err := w.WriteBits(uint64(len(param.Value)), 8); err != nil {
			return err
		}

		for i := 0; i < len(param.Value); i++ {
			if err := w.WriteBits(uint64(param.Value[i]), 8); err != nil {
				return err
			}
		}

		return nil
	}

	return w.WriteBits(0, 1)
}

func decodeQueryParam(r *BitReader) (QueryParam, error) {
	key, err := decodeQueryKey(r)
	if err != nil {
		return QueryParam{}, err
	}

	hasEq, err := r.ReadBits(1)
	if err != nil {
		return QueryParam{}, err
	}

	if hasEq == 0 {
		return QueryParam{
			Key:   key,
			HasEq: false,
		}, nil
	}

	length, err := r.ReadBits(8)
	if err != nil {
		return QueryParam{}, err
	}

	data := make([]byte, length)

	for i := range data {
		value, err := r.ReadBits(8)
		if err != nil {
			return QueryParam{}, err
		}

		data[i] = byte(value)
	}

	return QueryParam{
		Key:   key,
		Value: string(data),
		HasEq: true,
	}, nil
}

func encodeQuery(w *BitWriter, query *Query) error {
	if !query.HasPrefix {
		return w.WriteBits(0, 5)
	}

	if len(query.Params) > 31 {
		return fmt.Errorf(
			"too many query parameters: %d",
			len(query.Params),
		)
	}

	if err := w.WriteBits(
		uint64(len(query.Params)),
		5,
	); err != nil {
		return err
	}

	for _, param := range query.Params {
		if err := encodeQueryParam(w, param); err != nil {
			return err
		}
	}

	return nil
}

func decodeQuery(r *BitReader) (*Query, error) {
	count, err := r.ReadBits(5)
	if err != nil {
		return nil, err
	}

	if count == 0 {
		return &Query{}, nil
	}

	query := &Query{
		HasPrefix: true,
		Params:    make([]QueryParam, 0, count),
	}

	for i := uint64(0); i < count; i++ {
		param, err := decodeQueryParam(r)
		if err != nil {
			return nil, err
		}

		query.Params = append(query.Params, param)
	}

	return query, nil
}
