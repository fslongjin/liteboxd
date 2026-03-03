package service

import "errors"

var (
	ErrSandboxNotFound            = errors.New("sandbox not found")
	ErrSandboxRestartNotSupported = errors.New("sandbox restart is only supported for persistence-enabled sandboxes")
	ErrSandboxRestartInvalidState = errors.New("sandbox is terminating or deleted")
)
