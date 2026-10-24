package apperr

type Kind int

const (
	KindInternal Kind = iota
	KindBadRequest
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
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

func Wrap(err error, kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message, err: err}
}

func New(Kind Kind, Message string) *Error {
	return &Error{Kind: Kind, Message: Message}
}
