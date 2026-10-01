package application

import "strings"

// ProviderImageInput carries the frozen identity and the first text-only image
// request. It cannot carry arbitrary tool commands, credentials, or local paths.
type ProviderImageInput struct {
	Identity ProviderDispatchIdentity
	Model    string
	Prompt   string
}

// Validate limits the first image protocol to a bound model and bounded prompt.
func (i ProviderImageInput) Validate() error {
	if i.Identity.Validate() != nil || i.Model == "" || strings.TrimSpace(i.Model) != i.Model || len(i.Model) > 200 || strings.TrimSpace(i.Prompt) == "" || len(i.Prompt) > 32*1024 {
		return ErrInvalidProviderCall
	}
	return nil
}

// ProviderImageResult separates generated evidence from delivery uncertainty.
// Completed does not establish cost, moderation, or a selected media asset.
type ProviderImageResult struct {
	Outcome     string
	FailureCode string
	Receipt     *ProviderReceipt
}
