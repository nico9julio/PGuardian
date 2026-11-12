package crypto

import (
	"encoding/base64"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var dpapiEntropy = []byte("PGuardian-MachineSecret-v1-Entropy")

// ProtectString encrypts a plaintext secret using Windows DPAPI bound to the local machine
// and returns a base64-encoded ciphertext safe for SQLite storage.
func ProtectString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	inBytes := []byte(plaintext)
	inBlob := windows.DataBlob{
		Size: uint32(len(inBytes)),
		Data: &inBytes[0],
	}
	entropyBlob := windows.DataBlob{
		Size: uint32(len(dpapiEntropy)),
		Data: &dpapiEntropy[0],
	}

	var outBlob windows.DataBlob
	flags := uint32(windows.CRYPTPROTECT_UI_FORBIDDEN | windows.CRYPTPROTECT_LOCAL_MACHINE)
	if err := windows.CryptProtectData(&inBlob, nil, &entropyBlob, 0, nil, flags, &outBlob); err != nil {
		return "", fmt.Errorf("dpapi encrypt failed: %w", err)
	}
	defer func() {
		if outBlob.Data != nil {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))
		}
	}()

	cipherSlice := unsafe.Slice(outBlob.Data, outBlob.Size)
	copied := make([]byte, len(cipherSlice))
	copy(copied, cipherSlice)
	return base64.StdEncoding.EncodeToString(copied), nil
}

// UnprotectString decrypts a base64-encoded DPAPI ciphertext back into plaintext.
func UnprotectString(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	cipherBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid base64 secret: %w", err)
	}
	if len(cipherBytes) == 0 {
		return "", errors.New("empty cipher payload")
	}

	inBlob := windows.DataBlob{
		Size: uint32(len(cipherBytes)),
		Data: &cipherBytes[0],
	}
	entropyBlob := windows.DataBlob{
		Size: uint32(len(dpapiEntropy)),
		Data: &dpapiEntropy[0],
	}

	var outBlob windows.DataBlob
	flags := uint32(windows.CRYPTPROTECT_UI_FORBIDDEN | windows.CRYPTPROTECT_LOCAL_MACHINE)
	if err := windows.CryptUnprotectData(&inBlob, nil, &entropyBlob, 0, nil, flags, &outBlob); err != nil {
		return "", fmt.Errorf("dpapi decrypt failed: %w", err)
	}
	defer func() {
		if outBlob.Data != nil {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))
		}
	}()

	plainSlice := unsafe.Slice(outBlob.Data, outBlob.Size)
	copied := make([]byte, len(plainSlice))
	copy(copied, plainSlice)
	return string(copied), nil
}
