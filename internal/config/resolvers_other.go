//go:build !linux && !darwin && !windows

package config

// SystemResolvers is unsupported on this platform.
func SystemResolvers() ([]string, error) {
	return nil, ErrSystemResolversUnavailable
}
