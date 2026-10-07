package codec

import "testing"

func TestParsePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		segments []string
		leading  bool
		trailing bool
	}{
		{
			name:     "simple path",
			input:    "/products/phones",
			segments: []string{"products", "phones"},
			leading:  true,
			trailing: false,
		},
		{
			name:     "trailing slash",
			input:    "/products/phones/",
			segments: []string{"products", "phones"},
			leading:  true,
			trailing: true,
		},
		{
			name:     "root",
			input:    "/",
			segments: nil,
			leading:  true,
			trailing: true,
		},
		{
			name:     "empty path",
			input:    "",
			segments: nil,
			leading:  false,
			trailing: false,
		},
		{
			name:     "no leading slash",
			input:    "products/phones",
			segments: []string{"products", "phones"},
			leading:  false,
			trailing: false,
		},
		{
			name:     "double slash",
			input:    "/products//phones",
			segments: []string{"products", "", "phones"},
			leading:  true,
			trailing: false,
		},
		{
			name:     "multiple leading slashes",
			input:    "//products/phones",
			segments: []string{"", "products", "phones"},
			leading:  true,
			trailing: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, err := parsePath(tc.input)
			if err != nil {
				t.Fatalf("parsePath() error = %v", err)
			}

			if path.Leading != tc.leading {
				t.Fatalf(
					"expected Leading=%v, got %v",
					tc.leading,
					path.Leading,
				)
			}

			if path.Trailing != tc.trailing {
				t.Fatalf(
					"expected Trailing=%v, got %v",
					tc.trailing,
					path.Trailing,
				)
			}

			if len(path.Segments) != len(tc.segments) {
				t.Fatalf(
					"expected %d segments, got %d",
					len(tc.segments),
					len(path.Segments),
				)
			}

			for i := range tc.segments {
				if path.Segments[i] != tc.segments[i] {
					t.Fatalf(
						"segment %d: expected %q, got %q",
						i,
						tc.segments[i],
						path.Segments[i],
					)
				}
			}
		})
	}
}

func TestPathString(t *testing.T) {
	tests := []string{
		"",
		"/",
		"/products",
		"/products/phones",
		"/products/phones/",
		"products/phones",
		"/products//phones",
		"//products/phones",
	}

	for _, expected := range tests {
		t.Run(expected, func(t *testing.T) {
			path, err := parsePath(expected)
			if err != nil {
				t.Fatalf("parsePath() error = %v", err)
			}

			got := path.String()

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

func TestPathRoundTrip(t *testing.T) {
	tests := []string{
		"/",
		"/a",
		"/a/b",
		"/a/b/c",
		"/a/b/",
		"/a//b",
		"//a/b",
		"products/phones",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			path, err := parsePath(input)
			if err != nil {
				t.Fatalf("parsePath() error = %v", err)
			}

			if got := path.String(); got != input {
				t.Fatalf(
					"round trip failed: expected %q, got %q",
					input,
					got,
				)
			}
		})
	}
}

func TestPathCodec(t *testing.T) {
	tests := []string{
		"",
		"/",
		"/api",
		"/api/v1",
		"/users/12345",
		"/products/iphone-17",
		"/api/v2/users/12345",
		"/products//phones",
		"/products/phones/",
		"products/phones",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			path, err := parsePath(input)
			if err != nil {
				t.Fatalf(
					"parsePath() failed: %v",
					err,
				)
			}

			writer := NewBitWriter()

			if err := encodePath(writer, path); err != nil {
				t.Fatalf(
					"encodePath() failed: %v",
					err,
				)
			}

			reader := NewBitReader(
				writer.Bytes(),
				writer.BitLen(),
			)

			decoded, err := decodePath(reader)
			if err != nil {
				t.Fatalf(
					"decodePath() failed: %v",
					err,
				)
			}

			got := decoded.String()

			if got != input {
				t.Fatalf(
					"expected %q, got %q",
					input,
					got,
				)
			}

			if reader.Remaining() != 0 {
				t.Fatalf(
					"expected 0 remaining bits, got %d",
					reader.Remaining(),
				)
			}
		})
	}
}
