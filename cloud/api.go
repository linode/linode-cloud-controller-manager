package cloud

import "errors"

var (
	ErrNotImplemented = errors.New("not implemented")
	ErrLBUnsupported  = errors.New("loadbalancer unsupported")
)
