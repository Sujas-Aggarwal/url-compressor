package codec

import "testing"

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		params []QueryParam
	}{
		{
			name:  "single parameter",
			input: "page=2",
			params: []QueryParam{
				{
					Key:   "page",
					Value: "2",
					HasEq: true,
				},
			},
		},
		{
			name:  "multiple parameters",
			input: "a=1&b=2&c=3",
			params: []QueryParam{
				{Key: "a", Value: "1", HasEq: true},
				{Key: "b", Value: "2", HasEq: true},
				{Key: "c", Value: "3", HasEq: true},
			},
		},
		{
			name:  "parameter without equals",
			input: "debug",
			params: []QueryParam{
				{
					Key:   "debug",
					Value: "",
					HasEq: false,
				},
			},
		},
		{
			name:  "empty value",
			input: "page=",
			params: []QueryParam{
				{
					Key:   "page",
					Value: "",
					HasEq: true,
				},
			},
		},
		{
			name:  "mixed parameters",
			input: "foo=1&debug&bar=",
			params: []QueryParam{
				{Key: "foo", Value: "1", HasEq: true},
				{Key: "debug", Value: "", HasEq: false},
				{Key: "bar", Value: "", HasEq: true},
			},
		},
		{
			name:  "empty parameter",
			input: "a=1&&b=2",
			params: []QueryParam{
				{Key: "a", Value: "1", HasEq: true},
				{Key: "", Value: "", HasEq: false},
				{Key: "b", Value: "2", HasEq: true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query, err := parseQuery(tc.input)
			if err != nil {
				t.Fatalf("parseQuery() error = %v", err)
			}

			if !query.HasPrefix {
				t.Fatal("expected query to exist")
			}

			if len(query.Params) != len(tc.params) {
				t.Fatalf(
					"expected %d params, got %d",
					len(tc.params),
					len(query.Params),
				)
			}

			for i, expected := range tc.params {
				actual := query.Params[i]

				if actual.Key != expected.Key {
					t.Errorf(
						"param %d key: expected %q, got %q",
						i,
						expected.Key,
						actual.Key,
					)
				}

				if actual.Value != expected.Value {
					t.Errorf(
						"param %d value: expected %q, got %q",
						i,
						expected.Value,
						actual.Value,
					)
				}

				if actual.HasEq != expected.HasEq {
					t.Errorf(
						"param %d HasEq: expected %v, got %v",
						i,
						expected.HasEq,
						actual.HasEq,
					)
				}
			}
		})
	}
}

func TestQueryString(t *testing.T) {
	tests := []string{
		"page=2",
		"a=1&b=2&c=3",
		"debug",
		"page=",
		"foo=1&debug&bar=",
		"a=1&&b=2",
	}

	for _, expected := range tests {
		t.Run(expected, func(t *testing.T) {
			query, err := parseQuery(expected)
			if err != nil {
				t.Fatalf("parseQuery() error = %v", err)
			}

			got := query.String()

			if got != expected {
				t.Fatalf(
					"expected %q, got %q",
					expected,
					got,
				)
			}
		})
	}
}

func TestEmptyQuery(t *testing.T) {
	query, err := parseQuery("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if query.HasPrefix {
		t.Fatal("empty query should not exist")
	}

	if len(query.Params) != 0 {
		t.Fatalf(
			"expected 0 params, got %d",
			len(query.Params),
		)
	}

	if query.String() != "" {
		t.Fatalf(
			"expected empty string, got %q",
			query.String(),
		)
	}
}

func TestQueryRoundTrip(t *testing.T) {
	tests := []string{
		"a=1",
		"a=1&b=2",
		"foo=bar&debug",
		"page=",
		"a=1&&b=2",
		"utm_source=google&utm_medium=cpc&page=2",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			query, err := parseQuery(input)
			if err != nil {
				t.Fatalf("parseQuery() error = %v", err)
			}

			if got := query.String(); got != input {
				t.Fatalf(
					"round trip failed: expected %q, got %q",
					input,
					got,
				)
			}
		})
	}
}
