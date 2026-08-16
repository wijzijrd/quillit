package handler

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// Spec §2 limits (docs/superpowers/specs/2026-08-16-cli-project-import-design.md).
const (
	maxTarFiles  = 1000
	maxAssetSize = 10 << 20 // matches UploadImage's multipart cap
)

type importItem struct {
	Slug          string
	DirectoryPath string
	Body          []byte
	Assets        []importAsset
}

type importAsset struct {
	Name string
	Data []byte
}

type tarballError struct{ msg string }

func (e *tarballError) Error() string { return e.msg }

// ignoredImportFile reports whether a tar member is CLI render scaffolding
// or config the import deliberately skips (spec §2).
func ignoredImportFile(base string) bool {
	if strings.HasPrefix(base, ".") {
		return true
	}
	switch {
	case base == "links.conf", base == "quillit.yaml":
		return true
	}
	switch path.Ext(base) {
	case ".html", ".css", ".js":
		return true
	}
	return false
}

// readImportTarball decodes a gzipped tar of a CLI project directory into
// entry items. An entry folder is a directory containing <folder-name>.md
// (mirrors cli/internal/entry.WalkAll's definition).
func readImportTarball(r io.Reader) ([]importItem, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, &tarballError{msg: "body is not a gzipped tarball"}
	}
	defer gz.Close()

	type folder struct {
		md     []byte
		hasMD  bool
		assets []importAsset
	}
	folders := map[string]*folder{} // clean dir path -> contents

	tr := tar.NewReader(gz)
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, &tarballError{msg: fmt.Sprintf("reading tarball: %v", err)}
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, &tarballError{msg: fmt.Sprintf("unsupported tar entry type for %q", hdr.Name)}
		}
		count++
		if count > maxTarFiles {
			return nil, &tarballError{msg: fmt.Sprintf("tarball has more than %d files", maxTarFiles)}
		}

		name := hdr.Name
		if strings.HasPrefix(name, "/") {
			return nil, &tarballError{msg: fmt.Sprintf("absolute path in tarball: %q", name)}
		}
		clean := path.Clean(name)
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, &tarballError{msg: fmt.Sprintf("path escapes archive root: %q", name)}
		}

		dir, base := path.Split(clean)
		dir = strings.Trim(dir, "/")
		if dir == "" {
			continue // root-level file (quillit.yaml etc.) — never an entry member
		}
		if ignoredImportFile(base) {
			continue
		}

		if hdr.Size > maxAssetSize {
			return nil, &tarballError{msg: fmt.Sprintf("%s exceeds the %d MB per-file limit", clean, maxAssetSize>>20)}
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxAssetSize+1))
		if err != nil {
			return nil, &tarballError{msg: fmt.Sprintf("reading %s: %v", clean, err)}
		}
		if len(data) > maxAssetSize {
			return nil, &tarballError{msg: fmt.Sprintf("%s exceeds the %d MB per-file limit", clean, maxAssetSize>>20)}
		}

		f := folders[dir]
		if f == nil {
			f = &folder{}
			folders[dir] = f
		}
		if base == path.Base(dir)+".md" {
			f.md = data
			f.hasMD = true
		} else {
			f.assets = append(f.assets, importAsset{Name: base, Data: data})
		}
	}

	dirs := make([]string, 0, len(folders))
	for d, f := range folders {
		if f.hasMD {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs) // deterministic report order
	items := make([]importItem, 0, len(dirs))
	for _, d := range dirs {
		f := folders[d]
		parent, slug := path.Split(d)
		items = append(items, importItem{
			Slug:          slug,
			DirectoryPath: strings.Trim(parent, "/"),
			Body:          f.md,
			Assets:        f.assets,
		})
	}
	return items, nil
}
