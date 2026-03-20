package mining

import cx "colossusx/colossusx"

type RuntimeState interface {
	CUDADeviceOrdinal() (int, bool)
	OpenCLContext() (OpenCLContext, bool)
}

type runtimeBackend interface {
	cx.HashBackend
	RuntimeState
	InitializeRuntime() error
}

func InitializeBackendRuntime(backend cx.HashBackend) (RuntimeState, error) {
	if rb, ok := backend.(runtimeBackend); ok {
		if err := rb.InitializeRuntime(); err != nil {
			return nil, err
		}
		return rb, nil
	}
	return nil, nil
}
