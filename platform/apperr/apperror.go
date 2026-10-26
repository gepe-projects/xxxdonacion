package apperr

import "fmt"

type Kind int

const (
	KindInternal Kind = iota
	KindBadRequest
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
	KindTooManyRequests
)

func (k Kind) String() string {
	switch k {
	case KindInternal:
		return "internal"
	case KindBadRequest:
		return "bad_request"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	case KindTooManyRequests:
		return "too_many_requests"
	default:
		return "unknown"
	}
}

type Error struct {
	Kind             Kind
	Message          string
	ValidationErrors *[]FieldError
	err              error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.err }

type FieldError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message,omitempty"`
}

func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

func Wrap(err error, kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message, err: err}
}

func BadRequest(message string) *Error {
	return New(KindBadRequest, message)
}

func Unauthorized(message string) *Error {
	return New(KindUnauthorized, message)
}

func Forbidden(message string) *Error {
	return New(KindForbidden, message)
}

func NotFound(message string) *Error {
	return New(KindNotFound, message)
}

func Conflict(message string) *Error {
	return New(KindConflict, message)
}

func TooManyRequests(message string) *Error {
	return New(KindTooManyRequests, message)
}

// Internal membungkus cause dengan konteks aksi. Pesan user selalu generik
// supaya detail internal tidak bocor ke client.
func Internal(err error, action string) *Error {
	return Wrap(fmt.Errorf("%s: %w", action, err), KindInternal, "oops something went wrong")
}
