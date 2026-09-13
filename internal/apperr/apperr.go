package apperr

import (
	"errors"
	"fmt"
)

type Code string

const (
	ConfigMissing       Code = "CONFIG_MISSING"
	AccountNotFound     Code = "ACCOUNT_NOT_FOUND"
	DestinationNotFound Code = "DESTINATION_NOT_FOUND"
	ProfileNotFound     Code = "PROFILE_NOT_FOUND"
	AccountDisabled     Code = "ACCOUNT_DISABLED"
	DestinationDisabled Code = "DESTINATION_DISABLED"
	AuthRequired        Code = "AUTH_REQUIRED"
	AuthExpired         Code = "AUTH_EXPIRED"
	PermissionMissing   Code = "PERMISSION_MISSING"
	FileNotFound        Code = "FILE_NOT_FOUND"
	FileUnreadable      Code = "FILE_UNREADABLE"
	VideoInvalid        Code = "VIDEO_INVALID"
	DuplicateContent    Code = "DUPLICATE_CONTENT"
	QueueEmpty          Code = "QUEUE_EMPTY"
	QueuePaused         Code = "QUEUE_PAUSED"
	JobTimeout          Code = "JOB_TIMEOUT"
	DatabaseError       Code = "DATABASE_ERROR"
	KeychainError       Code = "KEYCHAIN_ERROR"
	DaemonUnavailable   Code = "DAEMON_UNAVAILABLE"
	NotImplemented      Code = "NOT_IMPLEMENTED"
	InvalidInput        Code = "INVALID_INPUT"
	AlreadyExists       Code = "ALREADY_EXISTS"

	R2UploadFailed            Code = "R2_UPLOAD_FAILED"
	R2DeleteFailed            Code = "R2_DELETE_FAILED"
	R2URLUnavailable          Code = "R2_URL_UNAVAILABLE"
	MetaRequestFailed         Code = "META_REQUEST_FAILED"
	MetaRateLimited           Code = "META_RATE_LIMITED"
	InstagramContainerFailed  Code = "INSTAGRAM_CONTAINER_FAILED"
	InstagramProcessingFailed Code = "INSTAGRAM_PROCESSING_FAILED"
	InstagramPublishFailed    Code = "INSTAGRAM_PUBLISH_FAILED"

	GoogleAuthFailed        Code = "GOOGLE_AUTH_FAILED"
	YouTubeChannelRequired  Code = "YOUTUBE_CHANNEL_REQUIRED"
	YouTubeChannelNotFound  Code = "YOUTUBE_CHANNEL_NOT_FOUND"
	YouTubeUploadFailed     Code = "YOUTUBE_UPLOAD_FAILED"
	YouTubeProcessingFailed Code = "YOUTUBE_PROCESSING_FAILED"
	YouTubeRateLimited      Code = "YOUTUBE_RATE_LIMITED"
	YouTubeQuotaExceeded    Code = "YOUTUBE_QUOTA_EXCEEDED"
	YouTubePublishFailed    Code = "YOUTUBE_PUBLISH_FAILED"
)

const (
	ExitOK         = 0
	ExitGeneric    = 1
	ExitUsage      = 2
	ExitNotFound   = 3
	ExitValidation = 4
	ExitAuth       = 5
	ExitPartial    = 6
	ExitDuplicate  = 7
)

type Error struct {
	Code      Code           `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
	Err       error          `json:"-"`
}

func (e *Error) Error() string {
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) ExitCode() int {
	switch e.Code {
	case InvalidInput:
		return ExitUsage
	case AccountNotFound, DestinationNotFound, ProfileNotFound, FileNotFound:
		return ExitNotFound
	case FileUnreadable, VideoInvalid:
		return ExitValidation
	case AuthRequired, AuthExpired, PermissionMissing, GoogleAuthFailed, KeychainError:
		return ExitAuth
	case DuplicateContent:
		return ExitDuplicate
	default:
		return ExitGeneric
	}
}

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(code Code, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

func Invalid(message string) *Error {
	return New(InvalidInput, message)
}

func NotImpl(feature string) *Error {
	return New(NotImplemented, feature+" is not implemented yet")
}

func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var pe *PrintedError
	if errors.As(err, &pe) {
		if pe.Err == nil {
			return ExitGeneric
		}
		if e, ok := As(pe.Err); ok {
			return e.ExitCode()
		}
		return ExitGeneric
	}
	if e, ok := As(err); ok {
		return e.ExitCode()
	}
	return ExitGeneric
}

// PrintedError means the command already wrote the error payload.
type PrintedError struct {
	Err error
}

func Printed(err error) error {
	if err == nil {
		return nil
	}
	return &PrintedError{Err: err}
}

func (e *PrintedError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *PrintedError) Unwrap() error { return e.Err }

func (e *PrintedError) ExitCode() int { return ExitCode(e.Err) }
