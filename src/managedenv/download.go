package managedenv

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

func shaFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func (m *Manager) download(ctx context.Context, a Asset, op *Operation) (string, error) {
	if len(a.SHA256) != 64 {
		return "", failure("environment_source_unavailable", "asset has no pinned checksum")
	}
	if _, e := hex.DecodeString(a.SHA256); e != nil {
		return "", failure("environment_source_unavailable", "invalid checksum")
	}
	path := filepath.Join(m.root, "cache", "downloads", a.SHA256)
	if err := ownedPath(m.root, path); err != nil {
		return "", err
	}
	err := m.locked(ctx, "asset-"+a.SHA256, func() error {
		if h, e := shaFile(path); e == nil && h == a.SHA256 {
			// Extraction follows after this short cache lock. Refresh its age so
			// maintenance cannot retire an old cache entry between those steps.
			return os.Chtimes(path, m.now(), m.now())
		}
		u, e := url.Parse(a.URL)
		if e != nil || u.User != nil || u.RawQuery != "" {
			return failure("environment_source_unavailable", "invalid asset URL")
		}
		if u.Scheme != "https" && !(m.allowFile && (u.Scheme == "file" || u.Scheme == "http")) {
			return failure("environment_source_unavailable", "HTTPS is required")
		}
		limit := a.MaxBytes
		if limit <= 0 {
			limit = 2 << 30
		}
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt) * time.Second):
				}
			}
			var reader io.ReadCloser
			total := int64(0)
			if u.Scheme == "file" {
				reader, e = os.Open(filepath.FromSlash(u.Path))
				if e != nil {
					return e
				}
			} else {
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
				if err != nil {
					return err
				}
				response, err := m.client.Do(request)
				if err != nil {
					e = err
					continue
				}
				if response.StatusCode != http.StatusOK {
					response.Body.Close()
					e = fmt.Errorf("asset download HTTP %d", response.StatusCode)
					if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
						return failure("environment_source_unavailable", e.Error())
					}
					if response.StatusCode < 500 && response.StatusCode != 429 {
						return failure("environment_download_failed", e.Error())
					}
					continue
				}
				reader = response.Body
				total = response.ContentLength
			}
			if total > limit {
				reader.Close()
				return failure("environment_download_failed", "asset exceeds size limit")
			}
			tmp, err := os.CreateTemp(filepath.Dir(path), ".download-"+a.SHA256+"-"+m.coreID+"-*")
			if err != nil {
				reader.Close()
				return err
			}
			op.State = "downloading"
			op.Message = "正在下载运行组件"
			op.TotalBytes = total
			op.DownloadedBytes = 0
			op.Downloads++
			if e = m.record(op); e != nil {
				tmp.Close()
				reader.Close()
				os.Remove(tmp.Name())
				return e
			}
			h := sha256.New()
			n, copyErr := io.Copy(io.MultiWriter(tmp, h, &downloadProgress{manager: m, op: op}), io.LimitReader(reader, limit+1))
			reader.Close()
			syncErr := tmp.Sync()
			tmp.Close()
			op.DownloadedBytes = n
			if copyErr != nil || syncErr != nil {
				os.Remove(tmp.Name())
				if ctx.Err() != nil {
					return ctx.Err()
				}
				e = copyErr
				if e == nil {
					e = syncErr
				}
				// A local write/fsync failure cannot improve by fetching the same
				// archive again. Preserve its errno for the public error classifier.
				var local *os.PathError
				if syncErr != nil || errors.As(e, &local) {
					return e
				}
				continue
			}
			if n > limit || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
				os.Remove(tmp.Name())
				return failure("environment_checksum_mismatch", "download did not match the pinned asset")
			}
			if e = os.Rename(tmp.Name(), path); e != nil {
				os.Remove(tmp.Name())
				return e
			}
			return m.record(op)
		}
		return failure("environment_download_failed", "download failed after bounded retries")
	})
	return path, err
}

type archiveLink struct {
	path, target string
	hard         bool
}

func extract(ctx context.Context, path, dest string, a Asset) error {
	canonical, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return err
	}
	dest = canonical
	maxExpanded := a.MaxExpandedBytes
	if maxExpanded <= 0 || maxExpanded > 8<<30 {
		maxExpanded = 8 << 30
	}
	var expanded int64
	count := 0
	links := []archiveLink{}
	entryPath := func(name string) (string, error) {
		name = strings.ReplaceAll(name, "\\", "/")
		name = strings.TrimPrefix(name, "./")
		if strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			return "", failure("environment_archive_invalid", "absolute archive path")
		}
		if a.StripPrefix != "" {
			prefix := strings.TrimSuffix(a.StripPrefix, "/")
			if name == prefix || name == prefix+"/" {
				return "", nil
			}
			if !strings.HasPrefix(name, prefix+"/") {
				return "", failure("environment_archive_invalid", "unexpected archive root")
			}
			name = strings.TrimPrefix(name, prefix+"/")
		}
		p := filepath.Join(dest, filepath.FromSlash(name))
		if !inside(dest, p) {
			return "", failure("environment_archive_invalid", "archive path escapes destination")
		}
		return p, nil
	}
	writeEntry := func(name string, size int64, mode os.FileMode, reader io.Reader, link string, hard bool) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		count++
		expanded += size
		if count > 150000 || size < 0 || expanded > maxExpanded {
			return failure("environment_archive_invalid", "expanded archive exceeds limit")
		}
		p, e := entryPath(name)
		if e != nil || p == "" {
			return e
		}
		if e = ownedPath(dest, p); e != nil {
			return e
		}
		if link != "" {
			var target string
			if hard {
				target, e = entryPath(link)
			} else {
				if filepath.IsAbs(link) || strings.Contains(link, ":") {
					return failure("environment_archive_invalid", "absolute archive link")
				}
				target = filepath.Join(filepath.Dir(p), filepath.FromSlash(link))
			}
			if e != nil || target == "" || !inside(dest, target) {
				return failure("environment_archive_invalid", "archive link escapes destination")
			}
			links = append(links, archiveLink{p, target, hard})
			return nil
		}
		if mode.IsDir() {
			return os.MkdirAll(p, 0700)
		}
		if !mode.IsRegular() {
			return failure("environment_archive_invalid", "special archive member")
		}
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600|(mode.Perm()&0111))
		if e != nil {
			return e
		}
		_, e = io.CopyN(f, reader, size)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		return closeErr
	}
	switch a.Format {
	case "binary":
		if len(a.Executables) != 1 {
			return failure("environment_archive_invalid", "binary asset needs exactly one entry")
		}
		for _, p := range a.Executables {
			f, e := os.Open(path)
			if e != nil {
				return e
			}
			info, e := f.Stat()
			if e != nil {
				f.Close()
				return e
			}
			e = writeEntry(p, info.Size(), 0755, f, "", false)
			f.Close()
			if e != nil {
				return e
			}
		}
	case "zip":
		z, e := zip.OpenReader(path)
		if e != nil {
			return e
		}
		defer z.Close()
		for _, f := range z.File {
			r, e := f.Open()
			if e != nil {
				return e
			}
			link := ""
			size := int64(f.UncompressedSize64)
			if f.Mode()&os.ModeSymlink != 0 {
				b, err := io.ReadAll(io.LimitReader(r, 4097))
				if err != nil || len(b) > 4096 {
					r.Close()
					return failure("environment_archive_invalid", "invalid link")
				}
				link = string(b)
				size = 0
			}
			e = writeEntry(f.Name, size, f.Mode(), r, link, false)
			r.Close()
			if e != nil {
				return e
			}
		}
	case "tar.gz", "tar.xz", "tar":
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		defer f.Close()
		var r io.Reader = f
		if a.Format == "tar.xz" {
			xr, err := xz.NewReader(f)
			if err != nil {
				return err
			}
			r = xr
		}
		if a.Format == "tar.gz" {
			gz, e := gzip.NewReader(f)
			if e != nil {
				return e
			}
			defer gz.Close()
			r = gz
		}
		tr := tar.NewReader(r)
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			if h.Typeflag == tar.TypeXHeader || h.Typeflag == tar.TypeXGlobalHeader {
				continue
			}
			e = writeEntry(h.Name, h.Size, h.FileInfo().Mode(), tr, h.Linkname, h.Typeflag == tar.TypeLink)
			if e != nil {
				return e
			}
		}
	default:
		return failure("environment_archive_invalid", "unsupported archive format")
	}
	// Links are created last, in dependency order. Framework bundles commonly
	// put Resources -> Versions/Current/Resources before Current -> 3.x.
	pending := links
	for len(pending) > 0 {
		next := []archiveLink{}
		for _, l := range pending {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := ownedPath(dest, l.path); err != nil {
				return err
			}
			resolved, err := filepath.EvalSymlinks(l.target)
			if err != nil {
				next = append(next, l)
				continue
			}
			if !inside(dest, resolved) {
				return failure("environment_archive_invalid", "archive link escapes destination")
			}
			if err = os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
				return err
			}
			if l.hard {
				err = os.Link(resolved, l.path)
			} else {
				relative, _ := filepath.Rel(filepath.Dir(l.path), resolved)
				err = os.Symlink(relative, l.path)
			}
			if err != nil {
				return err
			}
		}
		if len(next) == len(pending) {
			return failure("environment_archive_invalid", "unresolved archive link")
		}
		pending = next
	}

	return nil
}

type downloadProgress struct {
	manager *Manager
	op      *Operation
	last    time.Time
}

func (p *downloadProgress) Write(b []byte) (int, error) {
	p.op.DownloadedBytes += int64(len(b))
	if time.Since(p.last) > 500*time.Millisecond {
		p.last = time.Now()
		if err := p.manager.record(p.op); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}
