package codec

import (
	"strconv"
	"strings"
)

func encodeSemantic(u *URL) *EncodedURL {
	var flags Flags

	host := u.Host

	// www. is redundant information that can be represented by one bit.
	if strings.HasPrefix(strings.ToLower(host), "www.") {
		flags |= FlagHasWWW
		host = host[4:]
	}

	// Port
	var port uint16

	if u.Port != "" {
		if parsedPort, err := strconv.ParseUint(u.Port, 10, 16); err == nil {
			port = uint16(parsedPort)
			flags |= FlagHasPort
		}
	}

	if u.Path != "" {
		flags |= FlagHasPath
	}

	if u.Query != "" {
		flags |= FlagHasQuery
	}

	if u.Fragment != "" {
		flags |= FlagHasFragment
	}

	return &EncodedURL{
		Flags:    flags,
		Scheme:   strings.ToLower(u.Scheme),
		Host:     host,
		Port:     port,
		Path:     u.Path,
		Query:    u.Query,
		Fragment: u.Fragment,
	}
}

func decodeSemantic(e *EncodedURL) *URL {
	host := e.Host

	if e.Flags&FlagHasWWW != 0 {
		host = "www." + host
	}

	var port string

	if e.Flags&FlagHasPort != 0 {
		port = strconv.FormatUint(uint64(e.Port), 10)
	}

	var path string
	if e.Flags&FlagHasPath != 0 {
		path = e.Path
	}

	var query string
	if e.Flags&FlagHasQuery != 0 {
		query = e.Query
	}

	var fragment string
	if e.Flags&FlagHasFragment != 0 {
		fragment = e.Fragment
	}

	return &URL{
		Scheme:   e.Scheme,
		Host:     host,
		Port:     port,
		Path:     path,
		Query:    query,
		Fragment: fragment,
	}
}
