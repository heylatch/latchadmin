//go:build !windows

package secret

// On Linux the key file is protected by its 0600 permissions only.
func protect(b []byte) ([]byte, error) { return b, nil }

func unprotect(b []byte) ([]byte, error) { return b, nil }
