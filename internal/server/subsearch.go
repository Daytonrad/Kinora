package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/scanner"
	"github.com/ScotyLucas/kinora/internal/store"
)

// Subtitle search and download (OpenSubtitles). The files go to <data>/subtitles/<item>/,
// never next to the video: the media folders are mounted read-only.

func (s *Server) subtitleDir(itemID int64) string {
	return filepath.Join(s.cfg.DataDir, "subtitles", strconv.FormatInt(itemID, 10))
}

// subtitleQuery describes an item for the subtitle service.
func (s *Server) subtitleQuery(p *store.Profile, it *store.Item, langs []string) meta.SubtitleQuery {
	q := meta.SubtitleQuery{Langs: langs, Title: it.Title}
	if it.Year != nil {
		q.Year = *it.Year
	}
	if h, err := meta.OSHash(it.Path); err == nil {
		q.Hash = h
	}
	if it.Kind == "episode" {
		q.Episode = true
		if it.SeasonNum != nil {
			q.Season = *it.SeasonNum
		}
		if it.EpisodeNum != nil {
			q.EpNum = *it.EpisodeNum
		}
		q.Title = it.SeriesTitle
		if it.SeriesID != nil {
			if se, err := s.st.Item(*it.SeriesID, p); err == nil {
				if se.TmdbID != nil && *se.TmdbID > 0 {
					q.TmdbID = *se.TmdbID
				}
				q.Title = se.Title
			}
		}
		return q
	}
	q.ImdbID = it.ImdbID
	if it.TmdbID != nil && *it.TmdbID > 0 {
		q.TmdbID = *it.TmdbID
	}
	return q
}

func requestLangs(r *http.Request, p *store.Profile) []string {
	var langs []string
	seen := map[string]bool{}
	for _, l := range append(splitLangs(r.URL.Query().Get("langs")), splitLangs(p.SubLangs)...) {
		if n := scanner.NormalizeLang(l); n != "" && !seen[n] {
			seen[n] = true
			langs = append(langs, n)
		}
	}
	if len(langs) == 0 {
		langs = []string{"hun", "eng"}
	}
	return langs
}

func (s *Server) handleSubtitleSearch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it := s.playableItem(w, r, c)
	if it == nil {
		return
	}
	langs := requestLangs(r, c.Profile)
	if !s.OpenSubs.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "langs": langs, "results": []any{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.OpenSubs.Search(ctx, s.subtitleQuery(c.Profile, it, langs))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	// mark the ones already downloaded for this item
	type row struct {
		meta.SubtitleResult
		Have bool `json:"have,omitempty"`
	}
	rows := make([]row, 0, len(res))
	for _, x := range res {
		rows = append(rows, row{SubtitleResult: x})
	}
	if ids, err := s.downloadedFileIDs(it.ID); err == nil {
		for i := range rows {
			rows[i].Have = ids[strconv.FormatInt(rows[i].FileID, 10)]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "langs": langs, "results": rows})
}

func (s *Server) downloadedFileIDs(itemID int64) (map[string]bool, error) {
	rows, err := s.st.DB.Query(`SELECT file_id FROM item_subtitles WHERE item_id=? AND provider='opensubtitles'`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		out[id] = true
	}
	return out, rows.Err()
}

func (s *Server) handleSubtitleDownload(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		FileID          int64  `json:"file_id"`
		Lang            string `json:"lang"`
		Release         string `json:"release"`
		HearingImpaired bool   `json:"hearing_impaired"`
		Forced          bool   `json:"forced"`
	}
	if !decode(w, r, &in) {
		return
	}
	it := s.playableItem(w, r, c)
	if it == nil {
		return
	}
	if in.FileID <= 0 {
		writeErr(w, http.StatusBadRequest, "hiányzik a felirat azonosítója")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	d, err := s.OpenSubs.Download(ctx, in.FileID)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, meta.ErrSubsNotConfigured) {
			status = http.StatusConflict
		}
		writeErr(w, status, err.Error())
		return
	}
	dir := s.subtitleDir(it.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.internalErr(w, r, err)
		return
	}
	lang := scanner.NormalizeLang(in.Lang)
	path := filepath.Join(dir, fmt.Sprintf("os-%d.%s.srt", in.FileID, orStr(lang, "und")))
	if err := os.WriteFile(path, d.Data, 0o644); err != nil {
		s.internalErr(w, r, err)
		return
	}
	title := strings.TrimSpace(in.Release)
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80]) + "…"
	}
	sid, err := s.st.AddDownloadedSub(&store.DownloadedSub{ItemID: it.ID, Lang: lang, Title: title, Path: path,
		Provider: "opensubtitles", FileID: strconv.FormatInt(in.FileID, 10), HearingImpaired: in.HearingImpaired, Forced: in.Forced})
	if err != nil {
		os.Remove(path)
		s.internalErr(w, r, err)
		return
	}
	streams, err := s.st.Streams(it.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	for _, st := range streams {
		if st.DownloadID == sid {
			// the VTT conversion runs once here, so a broken file fails now and not in the player
			if _, err := s.Subs.VTT(ctx, it, st); err != nil {
				s.st.DeleteDownloadedSub(it.ID, sid)
				writeErr(w, http.StatusUnprocessableEntity, "a letöltött felirat nem olvasható: "+err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"stream": st, "remaining": d.Remaining})
			return
		}
	}
	s.internalErr(w, r, errors.New("a letöltött felirat nem jelent meg a sávok között"))
}

func (s *Server) handleSubtitleDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if !c.User.IsAdmin {
		writeErr(w, http.StatusForbidden, "letöltött feliratot csak admin törölhet")
		return
	}
	it, err := s.st.Item(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	if err := s.st.DeleteDownloadedSub(it.ID, pathID(r, "sid")); err != nil {
		writeErr(w, http.StatusNotFound, "nincs ilyen letöltött felirat")
		return
	}
	ok(w)
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
