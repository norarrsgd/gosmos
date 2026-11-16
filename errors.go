package gosmos

import (
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

var (
	ErrNotFound                  = errors.New("gosmos: not found")
	ErrConflict                  = errors.New("gosmos: conflict")
	ErrPreconditionFailed        = errors.New("gosmos: precondition failed")
	ErrValidation                = errors.New("gosmos: validation")
	ErrUnsupportedCrossPartition = errors.New("gosmos: unsupported cross-partition query")
)

// Error retains operation context and the original SDK error.
type Error struct {
	Op     string
	Entity string
	Status int
	Err    error
}

func (e *Error) Error() string { return fmt.Sprintf("gosmos %s %s: %v", e.Op, e.Entity, e.Err) }
func (e *Error) Unwrap() error { return e.Err }
func (e *Error) Is(target error) bool {
	return (e.Status == 404 && target == ErrNotFound) || (e.Status == 409 && target == ErrConflict) || (e.Status == 412 && target == ErrPreconditionFailed)
}
func wrapError(op, entity string, err error) error {
	if err == nil {
		return nil
	}
	e := &Error{Op: op, Entity: entity, Err: err}
	var response *azcore.ResponseError
	if errors.As(err, &response) {
		e.Status = response.StatusCode
	}
	return e
}
func validation(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}
