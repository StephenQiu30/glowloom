package application

// GenerationEstimate describes the current cost without authorizing a charge.
type GenerationEstimate struct {
	Credits        string `json:"credits"`
	MaximumCredits string `json:"maximum_credits,omitempty"`
	Kind           string `json:"kind"`
	Basis          string `json:"basis"`
	Quantity       string `json:"quantity,omitempty"`
	Reused         bool   `json:"reused,omitempty"`
}
