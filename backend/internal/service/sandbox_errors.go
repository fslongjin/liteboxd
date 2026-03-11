package service

import "errors"

var (
	ErrSandboxNotFound            = errors.New("sandbox not found")
	ErrSandboxRestartNotSupported = errors.New("sandbox restart is only supported for persistence-enabled sandboxes")
	ErrSandboxRestartInvalidState = errors.New("sandbox is terminating or deleted")
	ErrSandboxStopNotSupported    = errors.New("sandbox stop is only supported for persistence-enabled sandboxes")
	ErrSandboxStopInvalidState    = errors.New("sandbox cannot be stopped in its current state")
	ErrSandboxStartNotSupported   = errors.New("sandbox start is only supported for persistence-enabled sandboxes")
	ErrSandboxNotStopped          = errors.New("sandbox is not stopped")
	ErrSandboxAlreadyStopped      = errors.New("sandbox is already stopped")
)
