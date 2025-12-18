package shared

type ContextKeys int

const (
	ContextRequestIDHeader ContextKeys = iota
)

const RequestIDHeader = "X-Request-ID"
