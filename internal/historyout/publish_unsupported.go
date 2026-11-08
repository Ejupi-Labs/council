//go:build !linux && !darwin && !windows

package historyout

import "context"

type nativeRoot struct{}

func openNativeRoot(string) (nativeRoot, RootIdentity, error) {
	return nativeRoot{}, RootIdentity{}, ErrUnsupported
}
func (nativeRoot) close() error                    { return nil }
func (nativeRoot) identity() (RootIdentity, error) { return RootIdentity{}, ErrUnsupported }
func (nativeRoot) requireAbsent(string) error      { return ErrUnsupported }
func (nativeRoot) publish(context.Context, string, []byte, Result, func(string) error) (Result, error) {
	return Result{}, ErrUnsupported
}
