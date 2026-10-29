package domain

// Validate checks both director workspaces without changing the shared document.
func (c DirectorConfig) Validate() error {
	if !validDirector(c) {
		return ErrInvalidCommand
	}
	return nil
}
