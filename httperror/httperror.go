package httperror

import (
	"fmt"
	"net/http"
)

type Error struct {
	Status  int    // HTTP status code.
	Message string // User visible error message.
	Err     error  // Optional reason for the HTTP error.
}

func (err *Error) Error() string {
	if err.Err != nil {
		return fmt.Sprintf("status %d, reason %s", err.Status, err.Err.Error())
	}
	return fmt.Sprintf("status %d", err.Status)
}

// Convert converts any error to a *Error.
func Convert(err error) *Error {
	if err == nil {
		return nil
	}
	status := http.StatusInternalServerError
	message := "Internal Server Error"
	if e, ok := err.(*Error); ok {
		if e.Message != "" {
			return e
		}
		status = e.Status
		err = e.Err
		message = http.StatusText(status)
	}
	return &Error{
		Status:  status,
		Message: message,
		Err:     err,
	}
}

func standardError(status int) *Error {
	return &Error{Status: status, Message: http.StatusText(status)}
}

var (
	ErrBadRequest       = standardError(http.StatusBadRequest)
	ErrForbidden        = standardError(http.StatusForbidden)
	ErrMethodNotAllowed = standardError(http.StatusMethodNotAllowed)
	ErrNotFound         = standardError(http.StatusNotFound)
)
