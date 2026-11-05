package domain

const (
	// MaxModelRequestBytes bounds a complete current model transcript, including
	// base64 media and history. It does not permit unlimited image history.
	MaxModelRequestBytes = 8 << 20
	// MaxToolRequestBytes bounds a complete tool request independently of model transcripts.
	MaxToolRequestBytes = 1 << 20
)
