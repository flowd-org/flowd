// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"fmt"

	"github.com/flowd-org/flowd/internal/metrics"
	"github.com/flowd-org/flowd/internal/secrets"
)

// DefaultSecretProvider implements SecretProvider using internal/secrets.
type DefaultSecretProvider struct{}

func (DefaultSecretProvider) Prepare(runID string, binding *Binding) (map[string]string, func() error, error) {
	if binding == nil || len(binding.SecretNames) == 0 {
		return nil, nil, nil
	}
	closeBuffers := func() {
		for name, buf := range binding.SecretBuffers {
			if _, ok := binding.SecretNames[name]; ok {
				buf.Close()
			}
		}
	}
	secretDir, err := secrets.RunDir(runID)
	if err != nil {
		closeBuffers()
		return nil, nil, err
	}
	paths := make(map[string]string, len(binding.SecretNames))
	for name := range binding.SecretNames {
		value := ""
		if raw, ok := binding.Values[name]; ok && raw != nil {
			if s, ok := raw.(string); ok {
				value = s
			} else {
				value = fmt.Sprint(raw)
			}
		}
		if buf, ok := binding.SecretBuffers[name]; ok {
			path, err := secrets.CreateFile(secretDir, name, buf.Bytes())
			if err != nil {
				closeBuffers()
				return nil, nil, err
			}
			paths[name] = path
			continue
		}
		path, err := secrets.CreateFile(secretDir, name, []byte(value))
		if err != nil {
			closeBuffers()
			return nil, nil, err
		}
		paths[name] = path
	}

	handles := make(map[string]string, len(paths))
	var closers []func() error
	for name, path := range paths {
		handlePath, closeFn, err := secrets.ReadHandle(path)
		if err != nil {
			for _, closeFn := range closers {
				_ = closeFn()
			}
			closeBuffers()
			return nil, nil, err
		}
		handles[name] = handlePath
		if closeFn != nil {
			closers = append(closers, closeFn)
		}
	}
	metrics.RecordSecretHandleCreated(len(handles))

	cleanup := func() error {
		var firstErr error
		for _, closeFn := range closers {
			if err := closeFn(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		closeBuffers()
		if len(binding.SecretValues) > 0 {
			for i := range binding.SecretValues {
				binding.SecretValues[i] = ""
			}
			binding.SecretValues = nil
		}
		success := firstErr == nil
		metrics.RecordSecretHandleCleanup(len(handles), success)
		return firstErr
	}
	return handles, cleanup, nil
}
