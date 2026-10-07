package codec

import "testing"

func TestSchemeCodec(t *testing.T) {
	tests := []string{
		"https",
		"http",
		"ftp",
		"ws",
		"wss",
		"zoommtg",
		"spotify",
		"my-custom-protocol",
	}

	for _, expected := range tests {
		t.Run(expected, func(t *testing.T) {
			writer := NewBitWriter()

			if err := encodeScheme(writer, expected); err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			reader := NewBitReader(
				writer.Bytes(),
				writer.BitLen(),
			)

			actual, err := decodeScheme(reader)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if actual != expected {
				t.Fatalf(
					"expected %q, got %q",
					expected,
					actual,
				)
			}

			if reader.Remaining() != 0 {
				t.Fatalf(
					"expected no remaining bits, got %d",
					reader.Remaining(),
				)
			}
		})
	}
}
