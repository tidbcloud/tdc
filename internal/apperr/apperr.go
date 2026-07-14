package apperr

import (
	"errors"
	"fmt"
)

// Error is the structured error type rendered by the CLI boundary.
type Error struct {
	Code     string
	Category string
	ExitCode int
	Message  string
	Cause    error
}

type appErrorer interface {
	AppError() *Error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code, category string, exitCode int, message string) *Error {
	return &Error{
		Code:     code,
		Category: category,
		ExitCode: exitCode,
		Message:  message,
	}
}

func Wrap(code, category string, exitCode int, message string, cause error) *Error {
	return &Error{
		Code:     code,
		Category: category,
		ExitCode: exitCode,
		Message:  message,
		Cause:    cause,
	}
}

func NotImplemented(commandPath string) *Error {
	return New(
		"cli.not_implemented",
		"usage",
		2,
		fmt.Sprintf("%s is not implemented yet", commandPath),
	)
}

func ExitCodeFor(err error) int {
	if err == nil {
		return 0
	}

	var appErr *Error
	if errors.As(err, &appErr) && appErr.ExitCode > 0 {
		return appErr.ExitCode
	}

	var converted appErrorer
	if errors.As(err, &converted) {
		if appErr := converted.AppError(); appErr != nil && appErr.ExitCode > 0 {
			return appErr.ExitCode
		}
	}

	return 1
}

func CodeFor(err error) string {
	if err == nil {
		return ""
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	var converted appErrorer
	if errors.As(err, &converted) {
		if appErr := converted.AppError(); appErr != nil {
			return appErr.Code
		}
	}
	return ""
}

func CategoryFor(err error) string {
	if err == nil {
		return ""
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Category
	}
	var converted appErrorer
	if errors.As(err, &converted) {
		if appErr := converted.AppError(); appErr != nil {
			return appErr.Category
		}
	}
	return ""
}

func MessageFor(err error) string {
	if err == nil {
		return ""
	}

	var appErr *Error
	if errors.As(err, &appErr) && appErr.Message != "" {
		return appErr.Message
	}

	var converted appErrorer
	if errors.As(err, &converted) {
		if appErr := converted.AppError(); appErr != nil && appErr.Message != "" {
			return appErr.Message
		}
	}

	return err.Error()
}
