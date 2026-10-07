package codec

import "net/url"

type URL struct {
	Scheme   string
	Host     string
	Port     string
	Path     string
	Query    string
	Fragment string
}

type EncodedURL struct {
	Flags    Flags
	Scheme   string
	Host     string
	Port     uint16
	Path     string
	Query    string
	Fragment string
}

func Encode(raw string) ([]byte, uint64, error) {
	parsedURL, err := parse(raw)
	if err != nil {
		return nil, 0, err
	}

	// Normalize the URL into its semantic representation.
	encodedURL := encodeSemantic(parsedURL)

	writer := NewBitWriter()

	// ------------------------------------------------------------
	// Header
	// ------------------------------------------------------------

	// Codec version: 3 bits.
	// Version 0.
	if err := writer.WriteBits(0, 3); err != nil {
		return nil, 0, err
	}

	// URL flags: 8 bits.
	if err := writer.WriteBits(uint64(encodedURL.Flags), 8); err != nil {
		return nil, 0, err
	}

	// ------------------------------------------------------------
	// Scheme
	// ------------------------------------------------------------

	if err := encodeScheme(writer, encodedURL.Scheme); err != nil {
		return nil, 0, err
	}

	// ------------------------------------------------------------
	// Domain
	// ------------------------------------------------------------

	domain, err := parseDomain(encodedURL.Host)
	if err != nil {
		return nil, 0, err
	}

	if err := encodeDomain(writer, domain); err != nil {
		return nil, 0, err
	}

	// ------------------------------------------------------------
	// Port
	// ------------------------------------------------------------

	if encodedURL.Flags&FlagHasPort != 0 {
		if err := encodePort(writer, encodedURL.Port); err != nil {
			return nil, 0, err
		}
	}

	// ------------------------------------------------------------
	// Path
	// ------------------------------------------------------------

	if encodedURL.Flags&FlagHasPath != 0 {
		path, err := parsePath(encodedURL.Path)
		if err != nil {
			return nil, 0, err
		}

		if err := encodePath(writer, path); err != nil {
			return nil, 0, err
		}
	}

	// ------------------------------------------------------------
	// Query
	// ------------------------------------------------------------

	if encodedURL.Flags&FlagHasQuery != 0 {
		query, err := parseQuery(encodedURL.Query)
		if err != nil {
			return nil, 0, err
		}

		if err := encodeQuery(writer, query); err != nil {
			return nil, 0, err
		}
	}

	// ------------------------------------------------------------
	// Fragment
	// ------------------------------------------------------------

	if encodedURL.Flags&FlagHasFragment != 0 {
		if err := encodeFragment(writer, encodedURL.Fragment); err != nil {
			return nil, 0, err
		}
	}

	return writer.Bytes(), writer.BitLen(), nil
}

func parse(raw string) (*URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}

	return &URL{
		Scheme:   parsed.Scheme,
		Host:     parsed.Hostname(),
		Port:     parsed.Port(),
		Path:     parsed.Path,
		Query:    parsed.RawQuery,
		Fragment: parsed.Fragment,
	}, nil
}
