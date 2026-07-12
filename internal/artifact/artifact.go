package artifact

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/hash"
)

type File struct {
	Path          string `json:"path"`
	Mode          int    `json:"mode"`
	Content       []byte `json:"-"`
	Digest        string `json:"digest"`
	Deterministic bool   `json:"deterministic"`
}

type Manifest struct {
	BundleDigest string         `json:"bundleDigest"`
	Files        []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path          string `json:"path"`
	Mode          int    `json:"mode"`
	Digest        string `json:"digest"`
	Deterministic bool   `json:"deterministic"`
}

func Normalize(files []File) []File {
	out := make([]File, len(files))
	copy(out, files)
	for i := range out {
		out[i].Path = filepath.ToSlash(out[i].Path)
		out[i].Content = bytes.ReplaceAll(out[i].Content, []byte("\r\n"), []byte("\n"))
		out[i].Content = bytes.ReplaceAll(out[i].Content, []byte("\r"), []byte("\n"))
		if out[i].Mode == 0 {
			out[i].Mode = 0o644
		}
		out[i].Digest = hash.Bytes(out[i].Content)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func BundleDigest(files []File) string {
	normalized := Normalize(files)
	var buf bytes.Buffer
	for _, file := range normalized {
		if isSelfReferential(file.Path) {
			continue
		}
		buf.WriteString(file.Path)
		buf.WriteByte('\n')
		buf.WriteString(file.Digest)
		buf.WriteByte('\n')
	}
	return hash.Bytes(buf.Bytes())
}

func BuildManifest(files []File, bundleDigest string) Manifest {
	normalized := Normalize(files)
	manifest := Manifest{BundleDigest: bundleDigest, Files: make([]ManifestFile, 0, len(normalized))}
	for _, file := range normalized {
		if file.Path == "checksums.txt" {
			continue
		}
		manifest.Files = append(manifest.Files, ManifestFile{
			Path:          file.Path,
			Mode:          file.Mode,
			Digest:        file.Digest,
			Deterministic: file.Deterministic,
		})
	}
	return manifest
}

func Checksums(files []File) []byte {
	normalized := Normalize(files)
	var buf bytes.Buffer
	for _, file := range normalized {
		if file.Path == "checksums.txt" {
			continue
		}
		buf.WriteString(strings.TrimPrefix(file.Digest, "sha256:"))
		buf.WriteString("  ")
		buf.WriteString(file.Path)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func WriteFiles(ctx context.Context, root string, files []File) error {
	for _, file := range Normalize(files) {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, file.Content, os.FileMode(file.Mode)); err != nil {
			return err
		}
	}
	return nil
}

func isSelfReferential(path string) bool {
	return path == "checksums.txt" || path == "bundle.manifest.json" || path == "provenance.json"
}
