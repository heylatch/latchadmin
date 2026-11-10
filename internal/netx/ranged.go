package netx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// RangedFile reads a remote file with HTTP range requests, in blocks, so a zip's directory and a few of its
// entries can be read without downloading the whole archive. MafiaHub release zips carry the game client too;
// a server only needs a third of the bytes.
type RangedFile struct {
	ctx     context.Context
	url     string
	size    int64
	refresh func(context.Context) (string, error) // signed URLs expire; this gets a fresh one

	mu     sync.Mutex
	blocks map[int64][]byte
	order  []int64
	// Bytes fetched so far, for progress reporting.
	Fetched int64
}

const blockSize = 4 << 20
const keepBlocks = 4

// OpenRanged opens a remote file. refresh is called to get the URL and again when it has expired.
func OpenRanged(ctx context.Context, refresh func(context.Context) (string, error)) (*RangedFile, error) {
	u, err := refresh(ctx)
	if err != nil {
		return nil, err
	}
	f := &RangedFile{ctx: ctx, url: u, refresh: refresh, blocks: map[int64][]byte{}}
	res, err := f.get(0, 0)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	cr := res.Header.Get("Content-Range") // bytes 0-0/12345
	i := strings.LastIndexByte(cr, '/')
	if res.StatusCode != http.StatusPartialContent || i < 0 {
		return nil, errors.New("server does not support range requests")
	}
	if f.size, err = strconv.ParseInt(cr[i+1:], 10, 64); err != nil || f.size <= 0 {
		return nil, fmt.Errorf("bad Content-Range %q", cr)
	}
	return f, nil
}

// Size is the remote file's length.
func (f *RangedFile) Size() int64 { return f.size }

func (f *RangedFile) get(from, to int64) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		res, err := Get(f.ctx, f.url, http.Header{"Range": {fmt.Sprintf("bytes=%d-%d", from, to)}})
		if err != nil {
			return nil, err
		}
		// An expired signed URL answers 403 (or 400); get a new one once.
		if (res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusBadRequest) && attempt == 0 {
			res.Body.Close()
			if f.url, err = f.refresh(f.ctx); err != nil {
				return nil, err
			}
			continue
		}
		if res.StatusCode != http.StatusPartialContent {
			res.Body.Close()
			return nil, fmt.Errorf("range request: HTTP %d", res.StatusCode)
		}
		return res, nil
	}
}

func (f *RangedFile) block(n int64) ([]byte, error) {
	if b, ok := f.blocks[n]; ok {
		return b, nil
	}
	from := n * blockSize
	to := min(from+blockSize, f.size) - 1
	res, err := f.get(from, to)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, to-from+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != to-from+1 {
		return nil, io.ErrUnexpectedEOF
	}
	f.Fetched += int64(len(b))
	f.blocks[n] = b
	f.order = append(f.order, n)
	if len(f.order) > keepBlocks {
		delete(f.blocks, f.order[0])
		f.order = f.order[1:]
	}
	return b, nil
}

// ReadAt implements io.ReaderAt.
func (f *RangedFile) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if off >= f.size {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && off < f.size {
		b, err := f.block(off / blockSize)
		if err != nil {
			return n, err
		}
		c := copy(p[n:], b[off%blockSize:])
		n += c
		off += int64(c)
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
