package jfcompat

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// routes: lower-case patterns (ServeHTTP lower-cases the path). Both the older /Users/{user}/… forms and the
// newer user-less ones are served, since clients use either.
func (s *Server) routes() {
	m := s.mux
	pub, auth := s.public, s.authed
	m.HandleFunc("GET /system/info/public", pub(s.handleInfoPublic))
	m.HandleFunc("GET /system/info", auth(s.handleInfo))
	m.HandleFunc("GET /system/ping", pub(s.handlePing))
	m.HandleFunc("POST /system/ping", pub(s.handlePing))
	m.HandleFunc("GET /branding/configuration", pub(s.handleBranding))
	css := func(w http.ResponseWriter, r *http.Request, c *reqCtx) { w.Header().Set("Content-Type", "text/css") }
	m.HandleFunc("GET /branding/css", pub(css))
	m.HandleFunc("GET /branding/css.css", pub(css))

	m.HandleFunc("POST /users/authenticatebyname", pub(s.handleAuthByName))
	m.HandleFunc("POST /users/authenticatewithquickconnect", pub(s.handleAuthQC))
	m.HandleFunc("GET /users/public", pub(s.handleUsersPublic))
	m.HandleFunc("GET /users/me", auth(s.handleMe))
	m.HandleFunc("GET /users/{user}", auth(s.handleUser))
	m.HandleFunc("POST /sessions/logout", auth(s.handleLogout))
	m.HandleFunc("GET /quickconnect/enabled", pub(s.handleQCEnabled))
	m.HandleFunc("POST /quickconnect/initiate", pub(s.handleQCInitiate))
	m.HandleFunc("GET /quickconnect/initiate", pub(s.handleQCInitiate)) // before 10.8 it was a GET
	m.HandleFunc("GET /quickconnect/connect", pub(s.handleQCConnect))

	m.HandleFunc("GET /userviews", auth(s.handleViews))
	m.HandleFunc("GET /users/{user}/views", auth(s.handleViews))
	m.HandleFunc("GET /items", auth(s.handleItems))
	m.HandleFunc("GET /users/{user}/items", auth(s.handleItems))
	m.HandleFunc("GET /items/latest", auth(s.handleLatest))
	m.HandleFunc("GET /users/{user}/items/latest", auth(s.handleLatest))
	m.HandleFunc("GET /useritems/resume", auth(s.handleResume))
	m.HandleFunc("GET /users/{user}/items/resume", auth(s.handleResume))
	m.HandleFunc("GET /items/{item}", auth(s.handleItem))
	m.HandleFunc("GET /users/{user}/items/{item}", auth(s.handleItem))
	m.HandleFunc("GET /shows/nextup", auth(s.handleNextUp))
	m.HandleFunc("GET /shows/{item}/seasons", auth(s.handleSeasons))
	m.HandleFunc("GET /shows/{item}/episodes", auth(s.handleEpisodes))
	m.HandleFunc("GET /search/hints", auth(s.handleSearchHints))
	m.HandleFunc("GET /items/{item}/similar", auth(s.handleSimilar))
	m.HandleFunc("GET /movies/{item}/similar", auth(s.handleSimilar))
	m.HandleFunc("GET /shows/{item}/similar", auth(s.handleSimilar))
	m.HandleFunc("GET /genres", auth(s.handleGenres))
	for _, p := range []string{"/items/{item}/specialfeatures", "/users/{user}/items/{item}/specialfeatures",
		"/items/{item}/localtrailers", "/users/{user}/items/{item}/localtrailers"} {
		m.HandleFunc("GET "+p, auth(emptyList))
	}
	for _, p := range []string{"/items/{item}/intros", "/users/{user}/items/{item}/intros", "/studios", "/persons", "/artists",
		"/playlists", "/collections"} {
		m.HandleFunc("GET "+p, auth(emptyResult))
	}
	m.HandleFunc("GET /items/{item}/thememedia", auth(func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		empty := map[string]any{"Items": []any{}, "TotalRecordCount": 0, "StartIndex": 0}
		writeJSON(w, http.StatusOK, map[string]any{"ThemeVideosResult": empty, "ThemeSongsResult": empty, "SoundtrackSongsResult": empty})
	}))
	m.HandleFunc("GET /items/filters", auth(func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		writeJSON(w, http.StatusOK, map[string]any{"Genres": []any{}, "Tags": []any{}, "OfficialRatings": []any{}, "Years": []any{}})
	}))

	for _, p := range []string{"/items/{item}/images/{type}", "/items/{item}/images/{type}/{index}"} {
		m.HandleFunc("GET "+p, pub(s.handleImage))
		m.HandleFunc("HEAD "+p, pub(s.handleImage))
	}

	m.HandleFunc("POST /items/{item}/playbackinfo", auth(s.handlePlaybackInfo))
	m.HandleFunc("GET /items/{item}/playbackinfo", auth(s.handlePlaybackInfo))
	for _, p := range []string{"/items/{item}/download", "/items/{item}/file"} {
		m.HandleFunc("GET "+p, auth(s.handleStream))
		m.HandleFunc("HEAD "+p, auth(s.handleStream))
	}
	// stream, stream.mkv, master.m3u8, main.m3u8 (the mux has no partial-segment wildcards)
	m.HandleFunc("GET /videos/{item}/{file}", auth(s.handleVideoFile))
	m.HandleFunc("HEAD /videos/{item}/{file}", auth(s.handleVideoFile))
	m.HandleFunc("GET /videos/{item}/hls1/{session}/{file}", auth(s.handleSegment))
	m.HandleFunc("DELETE /videos/activeencodings", auth(s.handleStopEncodings))
	m.HandleFunc("GET /videos/{item}/{source}/subtitles/{index}/{file}", auth(s.handleSubtitle))
	m.HandleFunc("GET /videos/{item}/{source}/subtitles/{index}/{start}/{file}", auth(s.handleSubtitle))
	m.HandleFunc("GET /mediasegments/{item}", auth(s.handleSegments))

	m.HandleFunc("POST /sessions/playing", auth(s.handlePlaying(false)))
	m.HandleFunc("POST /sessions/playing/progress", auth(s.handlePlaying(false)))
	m.HandleFunc("POST /sessions/playing/stopped", auth(s.handlePlaying(true)))
	m.HandleFunc("POST /users/{user}/playingitems/{item}", auth(s.handlePlayingLegacy(false)))
	m.HandleFunc("POST /users/{user}/playingitems/{item}/progress", auth(s.handlePlayingLegacy(false)))
	m.HandleFunc("DELETE /users/{user}/playingitems/{item}", auth(s.handlePlayingLegacy(true)))
	m.HandleFunc("POST /playingitems/{item}", auth(s.handlePlayingLegacy(false)))
	m.HandleFunc("POST /playingitems/{item}/progress", auth(s.handlePlayingLegacy(false)))
	m.HandleFunc("DELETE /playingitems/{item}", auth(s.handlePlayingLegacy(true)))
	for _, p := range []string{"/userplayeditems/{item}", "/users/{user}/playeditems/{item}"} {
		m.HandleFunc("POST "+p, auth(s.handlePlayed(true)))
		m.HandleFunc("DELETE "+p, auth(s.handlePlayed(false)))
	}
	for _, p := range []string{"/userfavoriteitems/{item}", "/users/{user}/favoriteitems/{item}"} {
		m.HandleFunc("POST "+p, auth(s.handleFavorite(true)))
		m.HandleFunc("DELETE "+p, auth(s.handleFavorite(false)))
	}

	noContent := func(w http.ResponseWriter, r *http.Request, c *reqCtx) { w.WriteHeader(http.StatusNoContent) }
	m.HandleFunc("POST /sessions/playing/ping", auth(noContent))
	m.HandleFunc("POST /sessions/capabilities", auth(noContent))
	m.HandleFunc("POST /sessions/capabilities/full", auth(noContent))
	m.HandleFunc("GET /displaypreferences/{id}", auth(s.handleDisplayPrefs))
	m.HandleFunc("POST /displaypreferences/{id}", auth(noContent))
	for _, p := range []string{"/localization/cultures", "/localization/countries", "/localization/parentalratings", "/localization/options"} {
		m.HandleFunc("GET "+p, pub(emptyList))
	}
	m.HandleFunc("GET /playback/bitratetest", auth(s.handleBitrateTest))
	m.HandleFunc("GET /socket", auth(s.handleSocket))
}

func (s *Server) handleVideoFile(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	switch f := r.PathValue("file"); {
	case f == "stream" || strings.HasPrefix(f, "stream."):
		s.handleStream(w, r, c)
	case f == "master.m3u8" && r.Method == http.MethodGet:
		s.handleMaster(w, r, c)
	case f == "main.m3u8" && r.Method == http.MethodGet:
		s.handleMain(w, r, c)
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleDisplayPrefs(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, map[string]any{
		"Id": r.PathValue("id"), "SortBy": "SortName", "SortOrder": "Ascending", "RememberIndexing": false, "RememberSorting": false,
		"PrimaryImageHeight": 250, "PrimaryImageWidth": 250, "CustomPrefs": map[string]string{}, "ScrollDirection": "Horizontal",
		"ShowBackdrop": true, "ShowSidebar": false, "Client": qv(r, "Client"), "IndexBy": "",
	})
}

// handleBitrateTest sends Size bytes (at most 10 MB) so a client can measure the connection.
func (s *Server) handleBitrateTest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	n := qInt(r, "Size", 102400)
	if n <= 0 || n > 10<<20 {
		n = 10 << 20
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(n))
	buf := make([]byte, 32<<10)
	for n > 0 {
		k := min(n, len(buf))
		if _, err := w.Write(buf[:k]); err != nil {
			return
		}
		n -= k
	}
}

var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024, CheckOrigin: func(r *http.Request) bool { return true }}

// handleSocket keeps the clients' WebSocket open (keep-alives only): Kinora sends no remote-control messages.
func (s *Server) handleSocket(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.WriteJSON(map[string]any{"MessageType": "ForceKeepAlive", "Data": 60})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg struct {
				MessageType string `json:"MessageType"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.MessageType == "KeepAlive" {
				conn.WriteJSON(map[string]any{"MessageType": "KeepAlive"})
			}
		}
	}()
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-s.Ctx.Done():
			return
		case <-t.C:
		}
	}
}
