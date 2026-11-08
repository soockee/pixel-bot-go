package cloud

import (
	"bytes"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SaveToken keeps the refresh token in path, encrypted with DPAPI so only
// this Windows user on this machine can read it.
func SaveToken(path, token string) error {
	blob, err := dpapi([]byte(token), true)
	if err != nil {
		return err
	}
	return os.WriteFile(path, blob, 0o600)
}

// LoadToken reads a token SaveToken kept. It returns "" when there is none.
func LoadToken(path string) (string, error) {
	blob, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	token, err := dpapi(blob, false)
	return string(token), err
}

// ForgetToken removes the kept token.
func ForgetToken(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func dpapi(data []byte, protect bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty token")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var err error
	if protect {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size)), nil
}
