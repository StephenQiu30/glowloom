package authentication

import "golang.org/x/crypto/bcrypt"

const passwordHashCost = 12

type PasswordHasher struct{}

func NewPasswordHasher() PasswordHasher { return PasswordHasher{} }

func (PasswordHasher) Hash(password string) (string, error) {
	value, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	return string(value), err
}

func (PasswordHasher) Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
