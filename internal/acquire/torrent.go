package acquire

import (
	"context"
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxTorrentSize bounds a downloaded .torrent file.
const maxTorrentSize = 10 << 20

// MagnetHash returns the BitTorrent v1 info hash of a magnet link as lower-case hex.
func MagnetHash(magnet string) (string, bool) {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" {
		return "", false
	}
	for _, xt := range u.Query()["xt"] {
		const p = "urn:btih:"
		if !strings.HasPrefix(strings.ToLower(xt), p) {
			continue
		}
		h := xt[len(p):]
		switch len(h) {
		case 40:
			if _, err := hex.DecodeString(h); err == nil {
				return strings.ToLower(h), true
			}
		case 32:
			if b, err := base32.StdEncoding.DecodeString(strings.ToUpper(h)); err == nil {
				return hex.EncodeToString(b), true
			}
		}
	}
	return "", false
}

// InfoHash computes the info hash of a .torrent file: the SHA-1 of its bencoded "info" dictionary.
func InfoHash(torrent []byte) (string, error) {
	if len(torrent) == 0 || torrent[0] != 'd' {
		return "", errors.New("nem torrent fájl")
	}
	i := 1
	for i < len(torrent) && torrent[i] != 'e' {
		key, next, err := bstring(torrent, i)
		if err != nil {
			return "", err
		}
		end, err := bskip(torrent, next, 0)
		if err != nil {
			return "", err
		}
		if key == "info" {
			sum := sha1.Sum(torrent[next:end])
			return hex.EncodeToString(sum[:]), nil
		}
		i = end
	}
	return "", errors.New("a torrent fájlban nincs info szakasz")
}

// bstring reads a bencoded byte string at i: "4:spam".
func bstring(b []byte, i int) (string, int, error) {
	j := i
	for j < len(b) && b[j] >= '0' && b[j] <= '9' {
		j++
	}
	if j == i || j >= len(b) || b[j] != ':' || j-i > 9 {
		return "", 0, errors.New("hibás torrent fájl")
	}
	n := 0
	for _, c := range b[i:j] {
		n = n*10 + int(c-'0')
	}
	start := j + 1
	if start+n > len(b) {
		return "", 0, errors.New("hibás torrent fájl")
	}
	return string(b[start : start+n]), start + n, nil
}

// bskip returns the index just after the bencoded value at i.
func bskip(b []byte, i, depth int) (int, error) {
	if depth > 64 || i >= len(b) {
		return 0, errors.New("hibás torrent fájl")
	}
	switch c := b[i]; {
	case c == 'i':
		j := i + 1
		for j < len(b) && b[j] != 'e' {
			j++
		}
		if j >= len(b) {
			return 0, errors.New("hibás torrent fájl")
		}
		return j + 1, nil
	case c == 'l' || c == 'd':
		j := i + 1
		for j < len(b) && b[j] != 'e' {
			var err error
			if c == 'd' {
				if _, j, err = bstring(b, j); err != nil {
					return 0, err
				}
			}
			if j, err = bskip(b, j, depth+1); err != nil {
				return 0, err
			}
		}
		if j >= len(b) {
			return 0, errors.New("hibás torrent fájl")
		}
		return j + 1, nil
	case c >= '0' && c <= '9':
		_, j, err := bstring(b, i)
		return j, err
	}
	return 0, errors.New("hibás torrent fájl")
}

// Fetched is what a release link gave: a .torrent file or a magnet link.
type Fetched struct {
	Torrent  []byte
	Magnet   string
	InfoHash string
}

// FetchRelease resolves a result to something a download client can take, and learns its info hash
// so the download can be followed. The indexer's link (with its API key) never reaches the client.
func FetchRelease(ctx context.Context, hc *http.Client, r Result) (Fetched, error) {
	if r.Magnet != "" {
		h, ok := MagnetHash(r.Magnet)
		if !ok {
			return Fetched{}, errors.New("érvénytelen magnet link")
		}
		return Fetched{Magnet: r.Magnet, InfoHash: h}, nil
	}
	if r.Link == "" {
		return Fetched{}, errors.New("a kiadásnak nincs letöltési linkje")
	}
	if u, err := url.Parse(r.Link); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return Fetched{}, errors.New("a letöltési link http(s) legyen")
	}
	var magnet string
	c := *hc
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme == "magnet" {
			magnet = req.URL.String()
			return http.ErrUseLastResponse
		}
		if len(via) >= 5 {
			return errors.New("túl sok átirányítás")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", r.Link, nil)
	if err != nil {
		return Fetched{}, err
	}
	resp, err := c.Do(req)
	if err != nil {
		// some indexers redirect to the magnet with a Location the client cannot parse as a request
		if magnet == "" {
			return Fetched{}, fmt.Errorf("a torrent letöltése nem sikerült: %w", stripURL(err))
		}
	} else {
		defer resp.Body.Close()
		if magnet == "" {
			if loc := resp.Header.Get("Location"); strings.HasPrefix(loc, "magnet:") {
				magnet = loc
			}
		}
	}
	if magnet != "" {
		h, ok := MagnetHash(magnet)
		if !ok {
			return Fetched{}, errors.New("érvénytelen magnet link")
		}
		return Fetched{Magnet: magnet, InfoHash: h}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Fetched{}, fmt.Errorf("a torrent letöltése nem sikerült: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentSize+1))
	if err != nil {
		return Fetched{}, err
	}
	if len(body) > maxTorrentSize {
		return Fetched{}, errors.New("a torrent fájl túl nagy")
	}
	h, err := InfoHash(body)
	if err != nil {
		return Fetched{}, err
	}
	return Fetched{Torrent: body, InfoHash: h}, nil
}
