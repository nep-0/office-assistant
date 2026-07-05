package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var supportedOfficeExtensions = map[string]bool{
	".pdf":  true,
	".docx": true,
	".xlsx": true,
	".pptx": true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".tif":  true,
	".tiff": true,
	".webp": true,
}

func CleanFilename(filename string) string {
	cleaned := filepath.Base(strings.TrimSpace(filename))
	if cleaned == "." || cleaned == string(filepath.Separator) || cleaned == "" {
		return "upload"
	}
	return cleaned
}

func SupportedOfficeInput(filename string) bool {
	return supportedOfficeExtensions[strings.ToLower(filepath.Ext(filename))]
}

func StoragePath(storageRoot, token string) (string, string) {
	storageKey := filepath.Join("documents", token, "original")
	return storageKey, filepath.Join(storageRoot, storageKey)
}

func PrepareStoragePath(storageRoot, token string) (string, string, error) {
	storageKey, fullPath := StoragePath(storageRoot, token)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", "", err
	}
	return storageKey, fullPath, nil
}

func WriteUploadTemp(storageRoot string, file io.Reader) (string, string, int64, error) {
	if err := os.MkdirAll(filepath.Join(storageRoot, "tmp"), 0o755); err != nil {
		return "", "", 0, err
	}
	temp, err := os.CreateTemp(filepath.Join(storageRoot, "tmp"), "upload-*")
	if err != nil {
		return "", "", 0, err
	}
	defer temp.Close()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hasher), file)
	if err != nil {
		_ = os.Remove(temp.Name())
		return "", "", 0, err
	}
	if written == 0 {
		_ = os.Remove(temp.Name())
		return "", "", 0, errors.New("empty upload")
	}
	return temp.Name(), hex.EncodeToString(hasher.Sum(nil)), written, nil
}

func FTSQuery(query string) string {
	terms := strings.Fields(query)
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ReplaceAll(term, `"`, `""`)
		quoted = append(quoted, `"`+term+`"`)
	}
	return strings.Join(quoted, " ")
}
