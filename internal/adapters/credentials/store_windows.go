//go:build windows

package credentials

import (
	"context"
	"errors"
	"syscall"
	"unsafe"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
	"golang.org/x/sys/windows"
)

const (
	credTypeGeneric                       = 1
	credPersistLocalMachine               = 2
	errorNotFound           syscall.Errno = 1168
	errorCancelled          syscall.Errno = 1223
)

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

type nativeCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type Store struct{}

var _ application.CredentialStore = (*Store)(nil)

func NewStore() *Store { return &Store{} }

func (*Store) Store(ctx context.Context, id domain.CredentialID, secret []byte) error {
	if ctx.Err() != nil {
		return safeError("write", ErrInterrupted)
	}
	if id == "" || len(secret) == 0 || len(secret) > MaxSecretBytes {
		return safeError("write", application.ErrInvalidInput)
	}
	target, err := windows.UTF16PtrFromString(targetName(id))
	if err != nil {
		return safeError("write", application.ErrInvalidInput)
	}
	copyOfSecret := append([]byte(nil), secret...)
	defer wipe(copyOfSecret)
	credential := nativeCredential{Type: credTypeGeneric, TargetName: target, CredentialBlobSize: uint32(len(copyOfSecret)), CredentialBlob: &copyOfSecret[0], Persist: credPersistLocalMachine}
	result, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(&credential)), 0)
	if result == 0 {
		return mapNativeError("write", callErr)
	}
	return nil
}

func (*Store) Configured(ctx context.Context, id domain.CredentialID) (bool, error) {
	if ctx.Err() != nil {
		return false, safeError("status", ErrInterrupted)
	}
	credential, err := readNative("status", id)
	if errors.Is(err, ErrMissing) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	secret := unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))
	wipe(secret)
	procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	return true, nil
}

func (*Store) Delete(ctx context.Context, id domain.CredentialID) error {
	if ctx.Err() != nil {
		return safeError("delete", ErrInterrupted)
	}
	target, err := windows.UTF16PtrFromString(targetName(id))
	if err != nil {
		return safeError("delete", application.ErrInvalidInput)
	}
	result, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	if result == 0 {
		return mapNativeError("delete", callErr)
	}
	return nil
}

func (*Store) WithSecret(ctx context.Context, id domain.CredentialID, use func([]byte) error) error {
	if ctx.Err() != nil {
		return safeError("resolve", ErrInterrupted)
	}
	if use == nil {
		return safeError("resolve", application.ErrInvalidInput)
	}
	credential, err := readNative("resolve", id)
	if err != nil {
		return err
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	secret := unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))
	defer wipe(secret)
	if err := use(secret); err != nil {
		// Callback errors cross the secret boundary. Do not propagate arbitrary
		// text that may have formatted credential material.
		return safeError("use", application.ErrUnavailable)
	}
	return nil
}

func readNative(operation string, id domain.CredentialID) (*nativeCredential, error) {
	if id == "" {
		return nil, safeError(operation, application.ErrInvalidInput)
	}
	target, err := windows.UTF16PtrFromString(targetName(id))
	if err != nil {
		return nil, safeError(operation, application.ErrInvalidInput)
	}
	var credential *nativeCredential
	result, _, callErr := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&credential)))
	if result == 0 {
		return nil, mapNativeError(operation, callErr)
	}
	return credential, nil
}

func mapNativeError(operation string, err error) error {
	switch {
	case errors.Is(err, errorNotFound):
		return safeError(operation, ErrMissing)
	case errors.Is(err, syscall.ERROR_ACCESS_DENIED):
		return safeError(operation, ErrAccessDenied)
	case errors.Is(err, errorCancelled):
		return safeError(operation, ErrInterrupted)
	default:
		return safeError(operation, application.ErrUnavailable)
	}
}

func wipe(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
