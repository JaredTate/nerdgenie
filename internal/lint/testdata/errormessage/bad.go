package errormessage

import "errors"

// Fail returns an error whose message is too short to help anybody.
func Fail() error {
	return errors.New("bad input")
}
