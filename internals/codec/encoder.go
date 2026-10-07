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

func Encode(url string) string {
	parsedUrl, _ := parse(url)
	helper.CustomStructPrinter(parsedUrl)
	return "Started Encoding"
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
