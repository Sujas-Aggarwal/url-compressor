package codec

import (
	"net/url"

	"github.com/Sujas-Aggarwal/url-compressor/internals/helper"
)

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

func Encode(url string) string {
	parsedUrl, _ := parse(url)
	helper.CustomStructPrinter(parsedUrl)
	return url
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
