package codec

import (
	"strings"
)

type Path struct {
	Segments []string
	Leading  bool
	Trailing bool
}

func parsePath(raw string) (*Path, error) {
	if raw == "" {
		return &Path{}, nil
	}

	path := &Path{
		Leading:  strings.HasPrefix(raw, "/"),
		Trailing: strings.HasSuffix(raw, "/"),
	}

	trimmed := raw

	if path.Leading {
		trimmed = trimmed[1:]
	}

	if path.Trailing && trimmed != "" {
		trimmed = trimmed[:len(trimmed)-1]
	}

	if trimmed == "" {
		return path, nil
	}

	path.Segments = strings.Split(trimmed, "/")

	return path, nil
}

func (p *Path) String() string {
	if len(p.Segments) == 0 {
		if p.Leading {
			return "/"
		}

		return ""
	}

	var builder strings.Builder

	if p.Leading {
		builder.WriteByte('/')
	}

	builder.WriteString(strings.Join(p.Segments, "/"))

	if p.Trailing {
		builder.WriteByte('/')
	}

	return builder.String()
}
