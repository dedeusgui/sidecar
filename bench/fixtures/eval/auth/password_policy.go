package auth

import "errors"

// PasswordMinLength is the shortest password accepted at sign-up.
const PasswordMinLength = 8

// ValidatePassword rejects passwords shorter than the policy allows.
func ValidatePassword(pw string) error {
	if len(pw) < PasswordMinLength {
		return errors.New("password too short")
	}
	return nil
}
