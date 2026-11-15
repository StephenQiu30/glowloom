package http

import "github.com/gin-gonic/gin"

const legacyGovernanceErrorKey = "identity_legacy_governance_error_mapper"

// WithLegacyGovernanceErrors lets only the transitional mixed composition root
// translate its local Creation failures. Independent Identity adapters consume
// Identity-owned failures and never import another owner's application package.
func WithLegacyGovernanceErrors(mapper func(error) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if mapper != nil {
			c.Set(legacyGovernanceErrorKey, mapper)
		}
		c.Next()
	}
}
func mapLegacyGovernanceError(c *gin.Context, err error) error {
	value, exists := c.Get(legacyGovernanceErrorKey)
	if !exists {
		return err
	}
	mapper, ok := value.(func(error) error)
	if !ok || mapper == nil {
		return err
	}
	return mapper(err)
}
