package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLargeSBOMStreamsDeterministicSPDX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large fixture with spaces.spdx.json")
	digest, err := writeLargeSBOM(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() < 34<<20 || info.Size() >= (34<<20)+2048 {
		t.Fatalf("fixture must cross the old document cap without giant tokens: %v %v", info, err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	hash := sha256.New()
	decoder := json.NewDecoder(io.TeeReader(f, hash))
	if first, err := decoder.Token(); err != nil || first != json.Delim('{') {
		t.Fatal("missing SPDX document object", err)
	}
	metadata := map[string]any{}
	records := 0
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			t.Fatal(err)
		}
		if key != "files" {
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			metadata[key.(string)] = value
			continue
		}
		if first, err := decoder.Token(); err != nil || first != json.Delim('[') {
			t.Fatal("missing streamed files array", err)
		}
		for decoder.More() {
			var record struct {
				ID      string `json:"SPDXID"`
				Name    string `json:"fileName"`
				Comment string `json:"comment"`
			}
			if err := decoder.Decode(&record); err != nil {
				t.Fatal(err)
			}
			if record.ID != fmt.Sprintf("SPDXRef-SYNTHETIC-%06d", records) ||
				record.Name != fmt.Sprintf("./SYNTHETIC/record-%06d.txt", records) ||
				!strings.HasPrefix(record.Comment, "SYNTHETIC") || len(record.Comment) >= 2048 {
				t.Fatal("missing, duplicate or unbounded synthetic record")
			}
			records++
		}
		if _, err := decoder.Token(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		t.Fatalf("invalid document tail: %v", err)
	}
	if records < 30000 || records > 40000 {
		t.Fatalf("unexpected streamed record count: %d", records)
	}
	if metadata["spdxVersion"] != "SPDX-2.3" || metadata["SPDXID"] != "SPDXRef-DOCUMENT" ||
		metadata["dataLicense"] != "CC0-1.0" || !strings.Contains(metadata["comment"].(string), "SYNTHETIC TEST FIXTURE") {
		t.Fatal("SPDX standards marker or synthetic warning missing")
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest {
		t.Fatal("emitted checksum does not bind full document")
	}
	second, err := writeLargeSBOM(context.Background(), filepath.Join(t.TempDir(), "repeat.json"))
	if err != nil || second != digest {
		t.Fatal("large fixture is nondeterministic", err)
	}
	if _, err := writeLargeSBOM(context.Background(), path); err == nil {
		t.Fatal("existing fixture overwritten")
	}
}

func TestLargeSBOMCancellationRemovesOnlyItsPartialFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cancelled.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writeLargeSBOM(ctx, path); err == nil {
		t.Fatal("cancelled generation succeeded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("partial fixture survived cancellation")
	}
}
