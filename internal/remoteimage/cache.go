package remoteimage

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// DefaultCacheMaxBytes is the default size of the shared remote image cache.
const DefaultCacheMaxBytes = 256 << 20

const cacheHeaderSize = 17

type diskCache struct {
	dir   string
	limit int64
}

type cacheHeader struct {
	fetched, expires int64
	svg              bool
}

func readCacheHeader(file *os.File) (cacheHeader, error) {
	var b [cacheHeaderSize]byte
	_, err := io.ReadFull(file, b[:])
	return cacheHeader{
		fetched: int64(binary.LittleEndian.Uint64(b[:8])),
		expires: int64(binary.LittleEndian.Uint64(b[8:16])),
		svg:     b[16] == 1,
	}, err
}

func (c diskCache) withLock(ctx context.Context, fn func() error) (err error) {
	lock := flock.New(filepath.Join(c.dir, "cache.lock"))
	lockCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	locked, err := lock.TryLockContext(lockCtx, 10*time.Millisecond)
	if err != nil || !locked {
		return err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	return fn()
}

func (c diskCache) get(ctx context.Context, key string, now time.Time, freshAfter int64) (Image, bool) {
	if c.limit == 0 {
		return Image{}, false
	}
	var hit Image
	_ = c.withLock(ctx, func() error {
		path := filepath.Join(c.dir, key)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() <= cacheHeaderSize ||
			info.Size() > min(maxBytes+cacheHeaderSize, c.limit) {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		header, err := readCacheHeader(file)
		if err != nil || header.fetched == 0 || header.fetched < freshAfter || header.expires != 0 && header.expires <= now.UnixNano() {
			return err
		}
		data := make([]byte, info.Size()-cacheHeaderSize)
		if _, err := io.ReadFull(file, data); err != nil {
			return err
		}
		hit = Image{Data: data, SVG: header.svg}
		if info.ModTime().Before(now) {
			return os.Chtimes(path, now, now)
		}
		return nil
	})
	// A readable image can still be used when touching the file failed.
	return hit, len(hit.Data) != 0
}

func (c diskCache) trim(ctx context.Context) error {
	if c.limit == 0 {
		return nil
	}
	return c.withLock(ctx, func() error { return c.evict("", "", 0) })
}

func (c diskCache) put(ctx context.Context, key string, header cacheHeader, data []byte) error {
	size := int64(len(data) + cacheHeaderSize)
	if len(data) == 0 || len(data) > maxBytes || size > c.limit {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	return c.withLock(ctx, func() error {
		path := filepath.Join(c.dir, key)
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			current, err := os.Open(path)
			if err != nil {
				return err
			}
			old, readErr := readCacheHeader(current)
			closeErr := current.Close()
			if readErr == nil && closeErr == nil && old.fetched > header.fetched {
				return nil
			}
		}
		file, err := os.CreateTemp(c.dir, ".pending-")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		var b [cacheHeaderSize]byte
		binary.LittleEndian.PutUint64(b[:8], uint64(header.fetched))
		binary.LittleEndian.PutUint64(b[8:16], uint64(header.expires))
		if header.svg {
			b[16] = 1
		}
		_, headerErr := file.Write(b[:])
		_, dataErr := file.Write(data)
		if err := errors.Join(headerErr, dataErr, file.Close()); err != nil {
			return err
		}
		if err := c.evict(key, file.Name(), size); err != nil {
			return err
		}
		return os.Rename(file.Name(), path)
	})
}

func (c diskCache) evict(key, pending string, total int64) error {
	files, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	var entries []os.FileInfo
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, ".pending-") && name != filepath.Base(pending) {
			if err := os.Remove(filepath.Join(c.dir, name)); err != nil {
				return err
			}
			continue
		}
		stem, ok := strings.CutSuffix(name, ".image")
		if name == key || !ok || len(stem) != 64 || !file.Type().IsRegular() {
			continue
		}
		if _, err := hex.DecodeString(stem); err != nil {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return err
		}
		entries = append(entries, info)
		total += info.Size()
	}
	if total <= c.limit {
		return nil
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].ModTime().Before(entries[j].ModTime())
	})
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(c.dir, entry.Name())); err != nil {
			return err
		}
		total -= entry.Size()
		if total <= c.limit {
			break
		}
	}
	return nil
}
