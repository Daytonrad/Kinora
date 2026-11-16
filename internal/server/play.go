package server

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/scoty_lucas/kinora/internal/media"
	"github.com/scoty_lucas/kinora/internal/store"
)

func splitLangs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// playableItem loads a visible movie/episode with streams, or writes an error.
func (s *Server) playableItem(w http.ResponseWriter, r *http.Request, c *reqCtx) *store.Item {
	it, err := s.st.ItemFull(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return nil
	}
	if it.Kind != "movie" && it.Kind != "episode" {
		writeErr(w, http.StatusBadRequest, "ez az elem nem játszható le közvetlenül (sorozat vagy évad)")
		return nil
	}
	if it.Missing {
		writeErr(w, http.StatusConflict, "a fájl jelenleg nem érhető el (hiányzik vagy a megosztás nincs csatolva)")
		return nil
	}
	return it
}

func (s *Server) encoderSetting() string { return s.st.Setting("encoder") }

func (s *Server) handlePlayback(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var req media.PlaybackRequest
	if !decode(w, r, &req) {
		return
	}
	it := s.playableItem(w, r, c)
	if it == nil {
		return
	}
	if req.PrevSession != "" {
		if old := s.Media.Get(req.PrevSession, c.User.ID); old != nil {
			s.Media.Stop(old.ID)
		}
	}
	if req.Start < 0 || (it.RuntimeSec > 0 && req.Start >= float64(it.RuntimeSec)) {
		req.Start = 0
	}
	maxK := c.Profile.MaxBitrate
	if req.MaxBitrate > 0 && (maxK == 0 || req.MaxBitrate < maxK) {
		maxK = req.MaxBitrate
	}
	d := media.Decide(it, req, splitLangs(c.Profile.AudioLangs), maxK, s.Encoders.ToneMap())
	resp := map[string]any{
		"mode":          d.Mode,
		"reasons":       d.Reasons,
		"audio_idx":     d.AudioIdx,
		"streams":       it.Streams,
		"chapters":      it.Chapters,
		"duration":      it.RuntimeSec,
		"credits_start": it.CreditsStart,
		"start_offset":  0.0,
		"item":          map[string]any{"id": it.ID, "title": it.Title, "series_title": it.SeriesTitle, "season_num": it.SeasonNum, "episode_num": it.EpisodeNum, "kind": it.Kind, "series_id": it.SeriesID},
		"source": map[string]any{"container": it.Container, "video_codec": it.VideoCodec, "audio_codec": it.AudioCodec,
			"width": it.Width, "height": it.Height, "bitrate": it.Bitrate, "hdr": it.HDR},
	}
	if d.Mode == "direct" {
		resp["url"] = fmt.Sprintf("/api/stream/%d", it.ID)
		resp["encoder"] = ""
		writeJSON(w, http.StatusOK, resp)
		return
	}
	enc := s.Encoders.Pick(s.encoderSetting())
	sess, err := s.Media.Start(r.Context(), media.HLSSpec{Item: it, UserID: c.User.ID, Decision: d, Start: req.Start, Encoder: enc})
	if err != nil {
		s.log.Warn("playback failed", "item", it.Title, "mode", d.Mode, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "reasons": d.Reasons, "mode": d.Mode})
		return
	}
	resp["url"] = "/api/hls/" + sess.ID + "/index.m3u8"
	resp["session"] = sess.ID
	resp["start_offset"] = req.Start
	if d.VideoCopy {
		resp["encoder"] = "copy"
		resp["encoder_label"] = "nincs videó-átkódolás (copy)"
	} else {
		resp["encoder"] = enc.Name
		resp["encoder_label"] = enc.Label
		resp["bitrate"] = d.Bitrate
		resp["width"] = d.Width
	}
	writeJSON(w, http.StatusOK, resp)
}

var contentTypes = map[string]string{
	".mkv": "video/x-matroska", ".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime",
	".avi": "video/x-msvideo", ".ts": "video/mp2t", ".m2ts": "video/mp2t", ".wmv": "video/x-ms-wmv", ".flv": "video/x-flv",
	".mpg": "video/mpeg", ".mpeg": "video/mpeg", ".ogv": "video/ogg",
}

// contentDisposition builds an RFC 6266 header with an UTF-8 filename.
func contentDisposition(kind, name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, kind, ascii, strings.ReplaceAll(url.PathEscape(name), "'", "%27"))
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it := s.playableItem(w, r, c)
	if it == nil {
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		writeErr(w, http.StatusConflict, "a fájl nem olvasható: "+err.Error())
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	ext := strings.ToLower(filepath.Ext(it.Path))
	if ct := contentTypes[ext]; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if r.URL.Query().Get("dl") == "1" {
		w.Header().Set("Content-Disposition", contentDisposition("attachment", downloadName(it, "")+ext))
	}
	w.Header().Set("Cache-Control", "private, no-transform")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func downloadName(it *store.Item, quality string) string {
	name := it.Title
	if it.Kind == "episode" && it.SeriesTitle != "" {
		se := ""
		if it.SeasonNum != nil && it.EpisodeNum != nil {
			se = fmt.Sprintf(" S%02dE%02d", *it.SeasonNum, *it.EpisodeNum)
		}
		name = it.SeriesTitle + se + " - " + it.Title
	} else if it.Year != nil {
		name = fmt.Sprintf("%s (%d)", it.Title, *it.Year)
	}
	if quality != "" {
		name += " [" + quality + "]"
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
}

var reHLSFile = regexp.MustCompile(`^(index\.m3u8|seg\d{5,}\.ts)$`)

func (s *Server) handleHLS(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	file := r.PathValue("file")
	if strings.ContainsAny(file, `/\`) || strings.HasPrefix(file, ".") || !reHLSFile.MatchString(file) {
		writeErr(w, http.StatusBadRequest, "érvénytelen fájlnév")
		return
	}
	sess := s.Media.Get(r.PathValue("session"), c.User.ID)
	if sess == nil {
		writeErr(w, http.StatusNotFound, "a lejátszási munkamenet lejárt – indítsd újra a lejátszást")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if file == "index.m3u8" {
		b, err := sess.Playlist()
		if err != nil {
			writeErr(w, http.StatusNotFound, "a playlist még nem áll rendelkezésre")
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write(b)
		return
	}
	p := filepath.Join(sess.Dir, file)
	if _, err := os.Stat(p); err != nil {
		writeErr(w, http.StatusNotFound, "a szegmens még nem készült el")
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	http.ServeFile(w, r, p)
}

func (s *Server) handleHLSStop(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if sess := s.Media.Get(r.PathValue("session"), c.User.ID); sess != nil {
		s.Media.Stop(sess.ID)
	}
	ok(w)
}

func (s *Server) handleSubtitle(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it := s.playableItem(w, r, c)
	if it == nil {
		return
	}
	idx := int(pathID(r, "idx"))
	for _, st := range it.Streams {
		if st.Type == "subtitle" && st.TypeIndex == idx {
			p, err := s.Subs.VTT(r.Context(), it, st)
			if err != nil {
				writeErr(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			w.Header().Set("Cache-Control", "private, max-age=3600")
			http.ServeFile(w, r, p)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "nincs ilyen felirat")
}

// ------------------------------------------------------------------ downloads

func (s *Server) handleDownloadCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Quality  string `json:"quality"`
		AudioIdx *int   `json:"audio_idx"`
	}
	if !decode(w, r, &in) {
		return
	}
	it := s.playableItem(w, r, c)
	if it == nil {
		return
	}
	if in.Quality == "original" || in.Quality == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "done", "url": fmt.Sprintf("/api/stream/%d?dl=1", it.ID)})
		return
	}
	q, ok := media.QualityByName(in.Quality)
	if !ok {
		writeErr(w, http.StatusBadRequest, "ismeretlen minőség (1080p, 720p, 480p, 360p vagy original)")
		return
	}
	audio := media.ChooseAudio(it.Streams, in.AudioIdx, splitLangs(c.Profile.AudioLangs))
	job := s.Downloads.Enqueue(it, c.User.ID, q, audio, downloadName(it, q.Name)+".mp4")
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleDownloads(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	out := []media.Job{}
	for _, j := range s.Downloads.Jobs(c.User.ID) {
		if s.st.ItemVisible(j.ItemID, c.Profile) {
			out = append(out, j)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out, "qualities": media.Qualities})
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	j, path, found := s.Downloads.Get(r.PathValue("key"))
	if !found || j.UserID != c.User.ID || !s.st.ItemVisible(j.ItemID, c.Profile) {
		writeErr(w, http.StatusNotFound, "a letöltés nem található")
		return
	}
	if j.Status != "done" {
		writeErr(w, http.StatusConflict, "a letöltés még nem készült el")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, http.StatusGone, "a letöltött fájl már nem érhető el")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", contentDisposition("attachment", j.FileName))
	http.ServeContent(w, r, "", time.Unix(j.DoneAt, 0), f)
}

// ------------------------------------------------------------------ encoders

func (s *Server) handleHWTest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	res := s.Encoders.SelfTest(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"results": res, "selected": s.Encoders.Pick(s.encoderSetting()), "tonemap": s.Encoders.ToneMap()})
}
