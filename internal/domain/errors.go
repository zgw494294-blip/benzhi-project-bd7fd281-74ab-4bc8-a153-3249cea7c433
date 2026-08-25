package domain

import "fmt"

type ErrorCode string

const (
	CodeInvalid       ErrorCode = "INVALID_ARGUMENT"
	CodeNotFound      ErrorCode = "NOT_FOUND"
	CodeConflict      ErrorCode = "CONFLICT"
	CodeStaleRevision ErrorCode = "STALE_REVISION"
	CodeForbidden     ErrorCode = "FORBIDDEN"
	CodeFrozen        ErrorCode = "FROZEN"
	CodePrecondition  ErrorCode = "PRECONDITION_FAILED"
	CodeIntegrity     ErrorCode = "INTEGRITY_ERROR"
)

type Error struct {
	Code            ErrorCode
	Message         string
	CurrentRevision int64
	CurrentStatus   DatasetStatus
}

func (e *Error) Error() string { return e.Message }

func NewError(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func WithState(err *Error, d Dataset) *Error {
	err.CurrentRevision = d.Revision
	err.CurrentStatus = d.Status
	return err
}

func IsCode(err error, code ErrorCode) bool {
	de, ok := err.(*Error)
	return ok && de.Code == code
}
