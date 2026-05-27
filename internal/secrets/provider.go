package secrets

import (
	"os"
	"path/filepath"
	"strings"
)

type FileSecret struct {
	Path string
}

func (s FileSecret) Exists() bool {
	if strings.TrimSpace(s.Path) == "" {
		return false
	}
	info, err := os.Stat(s.Path)
	return err == nil && !info.IsDir()
}

func (s FileSecret) Read() (string, error) {
	value, err := os.ReadFile(s.Path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func (s FileSecret) SafeStatus() SecretStatus {
	return SecretStatus{
		Path:     filepath.Clean(s.Path),
		Exists:   s.Exists(),
		FileName: filepath.Base(s.Path),
	}
}

type SecretStatus struct {
	Path     string `json:"path"`
	FileName string `json:"fileName"`
	Exists   bool   `json:"exists"`
}
