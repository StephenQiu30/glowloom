package validation

import (
	"github.com/go-playground/validator/v10"
)

type Validator struct{ validator *validator.Validate }

func New() *Validator {
	return &Validator{validator: validator.New(validator.WithRequiredStructEnabled())}
}

func (value *Validator) Struct(target any) error {
	return value.validator.Struct(target)
}
