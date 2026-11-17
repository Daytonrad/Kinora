package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Stream tokens let <video src>, native players and <img> fetch media without
// headers. A token is an HMAC-signed claim set bound to user, profile, item and
// the device session that minted it; it expires after StreamTokenTTL and dies
// early when that device is logged out.
//
//	base64url("1|uid|pid|item|device|exp") "." base64url(hmac-sha256[:20])
//
// item 0 means "artwork only" (images of items the profile can see).
const StreamTokenTTL = 6 * time.Hour

type streamClaims struct {
	UserID, ProfileID, ItemID int64
	Device                    string
	Exp                       int64
}

var errStreamToken = errors.New("a lejátszási token érvénytelen vagy lejárt – indítsd újra a lejátszást")

func (s *Server) streamSecret() []byte {
	s.streamKeyOnce.Do(func() {
		k, _ := hex.DecodeString(s.st.Setting("stream_secret"))
		if len(k) < 32 {
			k = make([]byte, 32)
			rand.Read(k)
			s.st.SetSetting("stream_secret", hex.EncodeToString(k))
		}
		s.streamKey = k
	})
	return s.streamKey
}

func (s *Server) sign(payload string) string {
	m := hmac.New(sha256.New, s.streamSecret())
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:20])
}

func (s *Server) mintStreamToken(c *reqCtx, itemID int64) string {
	var pid int64
	if c.Profile != nil {
		pid = c.Profile.ID
	} else if c.ProfileLock > 0 {
		pid = c.ProfileLock // kids device before its PIN: filter as the locked profile
	}
	exp := time.Now().Add(StreamTokenTTL).Unix()
	if itemID == 0 {
		// artwork-only token: stable for a calendar day so image URLs stay cacheable
		day := int64(24 * 3600)
		exp = (time.Now().Unix()/day + 2) * day
	}
	p := fmt.Sprintf("1|%d|%d|%d|%s|%d", c.User.ID, pid, itemID, c.DeviceID, exp)
	return base64.RawURLEncoding.EncodeToString([]byte(p)) + "." + s.sign(p)
}

func (s *Server) parseStreamToken(tok string) (*streamClaims, error) {
	enc, sig, found := strings.Cut(tok, ".")
	if !found {
		return nil, errStreamToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return nil, errStreamToken
	}
	p := string(raw)
	if !hmac.Equal([]byte(sig), []byte(s.sign(p))) {
		return nil, errStreamToken
	}
	f := strings.Split(p, "|")
	if len(f) != 6 || f[0] != "1" {
		return nil, errStreamToken
	}
	var cl streamClaims
	var errs [4]error
	cl.UserID, errs[0] = strconv.ParseInt(f[1], 10, 64)
	cl.ProfileID, errs[1] = strconv.ParseInt(f[2], 10, 64)
	cl.ItemID, errs[2] = strconv.ParseInt(f[3], 10, 64)
	cl.Exp, errs[3] = strconv.ParseInt(f[5], 10, 64)
	cl.Device = f[4]
	if errors.Join(errs[:]...) != nil || cl.Exp < time.Now().Unix() || cl.Device == "" {
		return nil, errStreamToken
	}
	return &cl, nil
}

// media authorises a media request either by session (cookie/Bearer) or by a
// ?st= stream token. needProfile mirrors withProfile for session requests.
func (s *Server) media(needProfile bool, h handler) http.HandlerFunc {
	session := s.authed(h)
	if needProfile {
		session = s.withProfile(h)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		st := r.URL.Query().Get("st")
		if st == "" {
			session(w, r)
			return
		}
		cl, err := s.parseStreamToken(st)
		if err != nil || !s.st.DeviceActive(cl.Device, cl.UserID) {
			writeErr(w, http.StatusUnauthorized, errStreamToken.Error())
			return
		}
		u, err := s.st.UserByID(cl.UserID)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, errStreamToken.Error())
			return
		}
		c := &reqCtx{User: u, DeviceID: cl.Device, ViaStreamToken: true, StreamItem: cl.ItemID}
		if cl.ProfileID > 0 {
			p, err := s.st.Profile(cl.ProfileID)
			if err != nil || p.UserID != u.ID {
				writeErr(w, http.StatusUnauthorized, errStreamToken.Error())
				return
			}
			c.Profile = p
		}
		if needProfile && c.Profile == nil {
			writeErr(w, http.StatusPreconditionRequired, "válassz profilt")
			return
		}
		h(w, r, c)
	}
}

// withStreamToken appends ?st= to a relative API URL.
func withStreamToken(u, tok string) string {
	if tok == "" {
		return u
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + "st=" + url.QueryEscape(tok)
}

// rewritePlaylist appends the stream token to every segment URI of an HLS playlist.
func rewritePlaylist(b []byte, tok string) []byte {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" && !strings.HasPrefix(t, "#") {
			lines[i] = withStreamToken(t, tok)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
