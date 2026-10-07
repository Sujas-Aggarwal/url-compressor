package codec

import (
	"fmt"
	"strings"
)

func Decode(data []byte, bitLen uint64) (string, error) {
	if bitLen == 0 {
		return "", fmt.Errorf("cannot decode zero bits")
	}

	if uint64(len(data))*8 < bitLen {
		return "", fmt.Errorf(
			"invalid bit length: %d bits available, got %d",
			len(data)*8,
			bitLen,
		)
	}

	reader := NewBitReader(data, bitLen)

	// ------------------------------------------------------------
	// Header
	// ------------------------------------------------------------

	version, err := reader.ReadBits(3)
	if err != nil {
		return "", fmt.Errorf("read codec version: %w", err)
	}

	if version != 0 {
		return "", fmt.Errorf(
			"unsupported codec version: %d",
			version,
		)
	}

	rawFlags, err := reader.ReadBits(8)
	if err != nil {
		return "", fmt.Errorf("read flags: %w", err)
	}

	flags := Flags(rawFlags)

	// ------------------------------------------------------------
	// Scheme
	// ------------------------------------------------------------

	scheme, err := decodeScheme(reader)
	if err != nil {
		return "", fmt.Errorf("decode scheme: %w", err)
	}

	// ------------------------------------------------------------
	// Domain
	// ------------------------------------------------------------

	domain, err := decodeDomain(reader)
	if err != nil {
		return "", fmt.Errorf("decode domain: %w", err)
	}

	// ------------------------------------------------------------
	// Port
	// ------------------------------------------------------------

	var port uint16

	if flags&FlagHasPort != 0 {
		port, err = decodePort(reader)
		if err != nil {
			return "", fmt.Errorf("decode port: %w", err)
		}
	}

	// ------------------------------------------------------------
	// Path
	// ------------------------------------------------------------

	var path *Path

	if flags&FlagHasPath != 0 {
		path, err = decodePath(reader)
		if err != nil {
			return "", fmt.Errorf("decode path: %w", err)
		}
	}

	// ------------------------------------------------------------
	// Query
	// ------------------------------------------------------------

	var query *Query

	if flags&FlagHasQuery != 0 {
		query, err = decodeQuery(reader)
		if err != nil {
			return "", fmt.Errorf("decode query: %w", err)
		}
	}

	// ------------------------------------------------------------
	// Fragment
	// ------------------------------------------------------------

	var fragment string

	if flags&FlagHasFragment != 0 {
		fragment, err = decodeFragment(reader)
		if err != nil {
			return "", fmt.Errorf("decode fragment: %w", err)
		}
	}

	// ------------------------------------------------------------
	// Reconstruct URL
	// ------------------------------------------------------------

	var result strings.Builder

	// Current V0 format represents hierarchical URLs.
	if scheme != "" {
		result.WriteString(scheme)
		result.WriteString("://")
	}

	host := domain.String()

	if flags&FlagHasWWW != 0 {
		host = "www." + host
	}

	result.WriteString(host)

	if flags&FlagHasPort != 0 {
		fmt.Fprintf(&result, ":%d", port)
	}

	if flags&FlagHasPath != 0 && path != nil {
		result.WriteString(path.String())
	}

	if flags&FlagHasQuery != 0 && query != nil {
		result.WriteByte('?')
		result.WriteString(query.String())
	}

	if flags&FlagHasFragment != 0 {
		result.WriteByte('#')
		result.WriteString(fragment)
	}

	// Every bit in the payload should have been consumed.
	if remaining := reader.Remaining(); remaining != 0 {
		return "", fmt.Errorf(
			"trailing data: %d bits remain after decoding",
			remaining,
		)
	}

	return result.String(), nil
}
