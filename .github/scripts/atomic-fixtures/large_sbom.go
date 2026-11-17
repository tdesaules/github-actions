package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const largeSBOMBytes = 34 << 20

// Write records incrementally: neither the whole file nor the files array is
// retained in memory. Tokens remain below 2 KiB, exercising document size rather
// than the summarizer's separate oversized-token/depth rejection.
func writeLargeSBOM(ctx context.Context, path string) (digest string, err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	hash := sha256.New()
	writer := bufio.NewWriterSize(io.MultiWriter(f, hash), 64<<10)
	header := `{"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"SYNTHETIC summary-size regression","documentNamespace":"https://example.invalid/atomic-fixtures/large-sbom-v1","creationInfo":{"creators":["Tool: atomic-fixtures-SYNTHETIC"],"created":"2000-01-01T00:00:00Z"},"comment":"SYNTHETIC TEST FIXTURE: summary-size regression only; not a real package inventory.","files":[` + "\n"
	const footer = "]}\n"
	written, err := writer.WriteString(header)
	if err != nil {
		return "", err
	}
	comment := "SYNTHETIC bounded summary-size record. " + strings.Repeat("x", 768)
	for i := 0; written+len(footer) < largeSBOMBytes; i++ {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		record, encodeErr := json.Marshal(map[string]any{
			"SPDXID":           fmt.Sprintf("SPDXRef-SYNTHETIC-%06d", i),
			"fileName":         fmt.Sprintf("./SYNTHETIC/record-%06d.txt", i),
			"fileTypes":        []string{"TEXT"},
			"checksums":        []map[string]string{{"algorithm": "SHA256", "checksum": strings.Repeat("0", 64)}},
			"licenseConcluded": "NOASSERTION",
			"copyrightText":    "NOASSERTION",
			"comment":          comment,
		})
		if encodeErr != nil {
			return "", encodeErr
		}
		if i != 0 {
			if _, err = writer.WriteString(",\n"); err != nil {
				return "", err
			}
			written += 2
		}
		n, writeErr := writer.Write(record)
		if writeErr != nil {
			return "", writeErr
		}
		written += n
	}
	if _, err = writer.WriteString(footer); err != nil {
		return "", err
	}
	if err = writer.Flush(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = os.Chtimes(path, epoch, epoch); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
