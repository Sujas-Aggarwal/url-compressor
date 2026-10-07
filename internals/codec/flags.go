package codec

type Flags uint8

const (
	FlagHasPort Flags = 1 << iota
	FlagHasPath
	FlagHasQuery
	FlagHasFragment
	FlagHasWWW
)
