package jfcompat

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/media"
	"github.com/ScotyLucas/kinora/internal/store"
)

// ------------------------------------------------------------------ media sources

// Jellyfin numbers streams 0..n-1 over the file (external subtitles after the embedded ones); Kinora keeps them
// per type. jfStreams fixes one order, used both ways.
func jfStreams(it *store.Item) []store.StreamInfo {
	var out []store.StreamInfo
	for _, typ := range []string{"video", "audio", "subtitle"} {
		for _, st := range it.Streams {
			if st.Type == typ && !st.External {
				out = append(out, st)
			}
		}
	}
	for _, st := range it.Streams {
		if st.Type == "subtitle" && st.External {
			out = append(out, st)
		}
	}
	return out
}

// typeIndex maps a Jellyfin stream index to Kinora's index within the type (-1 when not that type).
func typeIndex(it *store.Item, jf int, typ string) int {
	l := jfStreams(it)
	if jf < 0 || jf >= len(l) || l[jf].Type != typ {
		return -1
	}
	return l[jf].TypeIndex
}

func jfIndexOf(it *store.Item, typ string, ti int) int {
	for i, st := range jfStreams(it) {
		if st.Type == typ && st.TypeIndex == ti {
			return i
		}
	}
	return -1
}

func videoRange(hdr string) (string, string) {
	switch hdr {
	case "HDR10":
		return "HDR", "HDR10"
	case "HLG":
		return "HDR", "HLG"
	case "Dolby Vision":
		return "HDR", "DOVI"
	case "":
		return "SDR", "SDR"
	}
	return "HDR", "HDR10"
}

func (s *Server) subtitleURL(it *store.Item, jf int, ext, token string) string {
	return fmt.Sprintf("/Videos/%s/%s/Subtitles/%d/0/Stream.%s?api_key=%s", s.itemID(it.ID), s.itemID(it.ID), jf, ext, token)
}

func (s *Server) mediaSource(it *store.Item, c *reqCtx) map[string]any {
	var streams []map[string]any
	defAudio, defSub := -1, -1
	for i, st := range jfStreams(it) {
		m := map[string]any{"Index": i, "Codec": st.Codec, "IsDefault": st.Default, "IsForced": st.Forced, "IsExternal": st.External,
			"IsInterlaced": false, "IsTextSubtitleStream": false, "SupportsExternalStream": false}
		if st.Lang != "" {
			m["Language"] = st.Lang
		}
		title := st.Title
		switch st.Type {
		case "video":
			m["Type"] = "Video"
			m["Width"], m["Height"] = it.Width, it.Height
			if it.Bitrate > 0 {
				m["BitRate"] = it.Bitrate
			}
			m["VideoRange"], m["VideoRangeType"] = videoRange(it.HDR)
			title = firstNonEmpty(title, fmt.Sprintf("%dp %s", it.Height, strings.ToUpper(st.Codec)))
		case "audio":
			m["Type"] = "Audio"
			if st.Channels > 0 {
				m["Channels"] = st.Channels
				m["ChannelLayout"] = map[int]string{1: "mono", 2: "stereo", 6: "5.1", 8: "7.1"}[st.Channels]
			}
			title = firstNonEmpty(title, strings.TrimSpace(st.Lang+" "+strings.ToUpper(st.Codec)))
			if defAudio < 0 || st.Default {
				if defAudio < 0 || st.Default {
					defAudio = i
				}
			}
		case "subtitle":
			m["Type"] = "Subtitle"
			m["IsTextSubtitleStream"] = st.TextBased
			m["SupportsExternalStream"] = st.TextBased
			if st.TextBased {
				m["DeliveryMethod"] = "External"
				m["DeliveryUrl"] = s.subtitleURL(it, i, "vtt", c.Token)
			} else {
				m["DeliveryMethod"] = "Embed"
			}
			title = firstNonEmpty(title, strings.TrimSpace(st.Lang+map[bool]string{true: " (forced)"}[st.Forced]))
			if st.Default && defSub < 0 {
				defSub = i
			}
		}
		m["DisplayTitle"] = firstNonEmpty(title, st.Codec)
		if st.Title != "" {
			m["Title"] = st.Title
		}
		streams = append(streams, m)
	}
	if streams == nil {
		streams = []map[string]any{}
	}
	src := map[string]any{
		"Protocol": "File", "Id": s.itemID(it.ID), "Path": "", "Type": "Default", "Container": it.Container, "Size": it.Size,
		"Name": it.Title, "IsRemote": false, "ETag": strconv.FormatInt(it.UpdatedAt, 10), "ReadAtNativeFramerate": false,
		"IgnoreDts": false, "IgnoreIndex": false, "GenPtsInput": false, "SupportsTranscoding": true, "SupportsDirectStream": true,
		"SupportsDirectPlay": true, "IsInfiniteStream": false, "RequiresOpening": false, "RequiresClosing": false,
		"RequiresLooping": false, "SupportsProbing": true, "VideoType": "VideoFile", "MediaStreams": streams,
		"MediaAttachments": []any{}, "Formats": []any{}, "RequiredHttpHeaders": map[string]string{},
		"DefaultAudioStreamIndex": defAudio, "HasSegments": false,
	}
	if it.RuntimeSec > 0 {
		src["RunTimeTicks"] = ticks(float64(it.RuntimeSec))
	}
	if it.Bitrate > 0 {
		src["Bitrate"] = it.Bitrate
	}
	if defSub >= 0 {
		src["DefaultSubtitleStreamIndex"] = defSub
	}
	return src
}

// ------------------------------------------------------------------ PlaybackInfo

// deviceProfile is the part of a client's DeviceProfile the decision needs.
type deviceProfile struct {
	DirectPlayProfiles []struct {
		Container  string `json:"Container"`
		Type       string `json:"Type"`
		VideoCodec string `json:"VideoCodec"`
		AudioCodec string `json:"AudioCodec"`
	} `json:"DirectPlayProfiles"`
}

type playbackInfoReq struct {
	UserID              string          `json:"UserId"`
	MaxStreamingBitrate int64           `json:"MaxStreamingBitrate"`
	StartTimeTicks      int64           `json:"StartTimeTicks"`
	AudioStreamIndex    *int            `json:"AudioStreamIndex"`
	SubtitleStreamIndex *int            `json:"SubtitleStreamIndex"`
	MediaSourceID       string          `json:"MediaSourceId"`
	EnableDirectPlay    *bool           `json:"EnableDirectPlay"`
	EnableDirectStream  *bool           `json:"EnableDirectStream"`
	EnableTranscoding   *bool           `json:"EnableTranscoding"`
	DeviceProfile       json.RawMessage `json:"DeviceProfile"`
}

var anyVideo = []string{"h264", "hevc", "vp9", "av1", "mpeg2video", "mpeg4", "vc1"}
var anyAudio = []string{"aac", "mp3", "ac3", "eac3", "dts", "truehd", "flac", "opus", "vorbis", "pcm_s16le", "alac"}
var anyContainer = []string{"mkv", "mp4", "mov", "webm", "ts", "avi"}

// capsFrom turns a DeviceProfile into Kinora's capabilities; no profile at all means "plays everything"
// (Infuse, Kodi and others play the original file and do not send one).
func capsFrom(raw json.RawMessage) media.Caps {
	if len(raw) == 0 || string(raw) == "null" {
		return media.Caps{Video: anyVideo, Audio: anyAudio, Containers: anyContainer, HDR: true}
	}
	var dp deviceProfile
	json.Unmarshal(raw, &dp)
	set := func(dst *[]string, list string, all []string) {
		if strings.TrimSpace(list) == "" {
			*dst = append(*dst, all...)
			return
		}
		for _, x := range strings.Split(strings.ToLower(list), ",") {
			x = strings.TrimSpace(x)
			switch x {
			case "h265":
				x = "hevc"
			case "matroska":
				x = "mkv"
			case "m4v":
				x = "mp4"
			case "mpegts":
				x = "ts"
			}
			if x != "" {
				*dst = append(*dst, x)
			}
		}
	}
	var c media.Caps
	for _, p := range dp.DirectPlayProfiles {
		if !strings.EqualFold(p.Type, "Video") {
			continue
		}
		set(&c.Containers, p.Container, anyContainer)
		set(&c.Video, p.VideoCodec, anyVideo)
		set(&c.Audio, p.AudioCodec, anyAudio)
	}
	// HDR: clients that can show it name the range types in their codec profiles
	b := bytes.ToUpper(raw)
	c.HDR = bytes.Contains(b, []byte("HDR10")) || bytes.Contains(b, []byte("DOVI"))
	return c
}

// playSession remembers a PlaybackInfo decision until the client fetches the stream.
type playSession struct {
	ItemID, UserID, ProfileID int64
	Decision                  media.Decision
	Start                     float64
	Kinora                    string // the Kinora HLS session once started
	created                   time.Time
}

type playSessions struct {
	mu sync.Mutex
	m  map[string]*playSession
}

func newPlaySessions() *playSessions { return &playSessions{m: map[string]*playSession{}} }

func (ps *playSessions) put(id string, p *playSession) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for k, v := range ps.m {
		if time.Since(v.created) > 12*time.Hour {
			delete(ps.m, k)
		}
	}
	p.created = time.Now()
	ps.m[id] = p
}

func (ps *playSessions) get(id string) *playSession {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.m[id]
}

func (ps *playSessions) remove(id string) *playSession {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p := ps.m[id]
	delete(ps.m, id)
	return p
}

// playable loads a visible film or episode with streams.
func (s *Server) playable(raw string, c *reqCtx) (*store.Item, bool) {
	id, ok := s.ids.decode(nsItem, raw)
	if !ok {
		return nil, false
	}
	it, err := s.Store.ItemFull(id, c.Profile)
	if err != nil || (it.Kind != "movie" && it.Kind != "episode") || it.Missing {
		return nil, false
	}
	return it, true
}

func (s *Server) handlePlaybackInfo(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.playable(r.PathValue("item"), c)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	var in playbackInfoReq
	if r.Method == http.MethodPost {
		if err := decode(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "bad request")
			return
		}
	}
	// the query string carries the same fields (and wins: some clients send both)
	if v := qInt(r, "MaxStreamingBitrate", 0); v > 0 {
		in.MaxStreamingBitrate = int64(v)
	}
	if v, err := strconv.ParseInt(qv(r, "StartTimeTicks"), 10, 64); err == nil {
		in.StartTimeTicks = v
	}
	for _, p := range []struct {
		name string
		dst  **int
	}{{"AudioStreamIndex", &in.AudioStreamIndex}, {"SubtitleStreamIndex", &in.SubtitleStreamIndex}} {
		if v, err := strconv.Atoi(qv(r, p.name)); err == nil {
			*p.dst = &v
		}
	}
	if b, set := qBool(r, "EnableDirectPlay"); set {
		in.EnableDirectPlay = &b
	}

	req := media.PlaybackRequest{Caps: capsFrom(in.DeviceProfile), Start: float64(in.StartTimeTicks) / 1e7}
	if in.AudioStreamIndex != nil {
		if ti := typeIndex(it, *in.AudioStreamIndex, "audio"); ti >= 0 {
			req.AudioIdx = &ti
		}
	}
	if in.SubtitleStreamIndex != nil {
		// an image subtitle cannot be sent as a file: it is burnt into a transcode
		if ti := typeIndex(it, *in.SubtitleStreamIndex, "subtitle"); ti >= 0 {
			for _, st := range it.Streams {
				if st.Type == "subtitle" && st.TypeIndex == ti && !st.TextBased && !st.External {
					req.BurnSubIdx = &ti
				}
			}
		}
	}
	if in.EnableDirectPlay != nil && !*in.EnableDirectPlay && in.EnableDirectStream != nil && !*in.EnableDirectStream {
		req.Force = "transcode"
	}
	maxK := c.Profile.MaxBitrate
	if k := int(in.MaxStreamingBitrate / 1000); k > 0 && (maxK == 0 || k < maxK) {
		maxK = k
	}
	if req.Start < 0 || (it.RuntimeSec > 0 && req.Start >= float64(it.RuntimeSec)) {
		req.Start = 0
	}
	tl := s.Store.TitleLangsFor(c.Profile.ID, store.TitleOf(it))
	d := media.Decide(it, req, media.WithTitleLang(tl.AudioLang, splitLangs(c.Profile.AudioLangs)), maxK, s.Encoders.ToneMap())

	src := s.mediaSource(it, c)
	if d.AudioIdx >= 0 {
		src["DefaultAudioStreamIndex"] = jfIndexOf(it, "audio", d.AudioIdx)
	}
	playID := store.RandomToken(16)
	if d.Mode != "direct" {
		s.plays.put(playID, &playSession{ItemID: it.ID, UserID: c.User.ID, ProfileID: c.Profile.ID, Decision: d, Start: req.Start})
		src["SupportsDirectPlay"], src["SupportsDirectStream"] = false, false
		src["TranscodingUrl"] = fmt.Sprintf("/Videos/%s/master.m3u8?MediaSourceId=%s&PlaySessionId=%s&api_key=%s", s.itemID(it.ID), s.itemID(it.ID), playID, c.Token)
		src["TranscodingSubProtocol"] = "hls"
		src["TranscodingContainer"] = map[bool]string{true: "mp4", false: "ts"}[d.FMP4]
		src["TranscodeReasons"] = transcodeReasons(it, req, d)
	}
	s.Log.Info("jellyfin-compat: playback", "item", it.Title, "mode", d.Mode, "client", c.Auth.Client, "reasons", strings.Join(d.Reasons, "; "))
	writeJSON(w, http.StatusOK, map[string]any{"MediaSources": []any{src}, "PlaySessionId": playID})
}

func splitLangs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// transcodeReasons picks the nearest of Jellyfin's fixed reason names (Kinora's own reasons are free text).
func transcodeReasons(it *store.Item, req media.PlaybackRequest, d media.Decision) []string {
	has := func(l []string, v string) bool {
		for _, x := range l {
			if strings.EqualFold(x, v) {
				return true
			}
		}
		return false
	}
	var out []string
	if !has(req.Caps.Containers, it.Container) {
		out = append(out, "ContainerNotSupported")
	}
	if !has(req.Caps.Video, it.VideoCodec) {
		out = append(out, "VideoCodecNotSupported")
	}
	if !has(req.Caps.Audio, it.AudioCodec) {
		out = append(out, "AudioCodecNotSupported")
	}
	if req.BurnSubIdx != nil {
		out = append(out, "SubtitleCodecNotSupported")
	}
	if it.HDR != "" && !req.Caps.HDR {
		out = append(out, "VideoRangeTypeNotSupported")
	}
	if d.Bitrate > 0 && it.Bitrate > d.Bitrate {
		out = append(out, "ContainerBitrateExceedsLimit")
	}
	if len(out) == 0 {
		out = []string{"DirectPlayError"}
	}
	return out
}

// ------------------------------------------------------------------ direct stream

var contentTypes = map[string]string{
	".mkv": "video/x-matroska", ".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime",
	".avi": "video/x-msvideo", ".ts": "video/mp2t", ".m2ts": "video/mp2t",
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.playable(r.PathValue("item"), c)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		s.Log.Warn("jellyfin-compat: file not readable", "item", it.ID, "err", err)
		writeErr(w, http.StatusConflict, "the file cannot be read")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "the file cannot be read")
		return
	}
	ext := strings.ToLower(filepath.Ext(it.Path))
	if ct := contentTypes[ext]; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if strings.Contains(r.URL.Path, "/download") {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, urlPathEscape(filepath.Base(it.Path))))
	}
	s.Media.TouchItem(c.User.ID, it.ID)
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func urlPathEscape(s string) string {
	var b strings.Builder
	for _, ch := range []byte(s) {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.IndexByte("-._~", ch) >= 0 {
			b.WriteByte(ch)
		} else {
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

// ------------------------------------------------------------------ HLS (Kinora's transcode sessions)

// session returns (starting it when needed) the Kinora HLS session of a play session. A client that opens
// master.m3u8 without a PlaybackInfo first gets a plain H.264/AAC transcode.
func (s *Server) session(r *http.Request, c *reqCtx, it *store.Item) (*media.Session, string, error) {
	playID := qv(r, "PlaySessionId")
	ps := s.plays.get(playID)
	if ps == nil || ps.ItemID != it.ID || ps.UserID != c.User.ID {
		req := media.PlaybackRequest{Caps: media.Caps{Video: []string{"h264"}, Audio: []string{"aac"}, Containers: []string{"ts"}}, Force: "transcode"}
		if v, err := strconv.ParseInt(qv(r, "StartTimeTicks"), 10, 64); err == nil && v > 0 {
			req.Start = float64(v) / 1e7
		}
		d := media.Decide(it, req, splitLangs(c.Profile.AudioLangs), c.Profile.MaxBitrate, s.Encoders.ToneMap())
		if playID == "" {
			playID = store.RandomToken(16)
		}
		ps = &playSession{ItemID: it.ID, UserID: c.User.ID, ProfileID: c.Profile.ID, Decision: d, Start: req.Start}
		s.plays.put(playID, ps)
	}
	if ps.Kinora != "" {
		if sess := s.Media.Get(ps.Kinora, c.User.ID); sess != nil {
			return sess, playID, nil
		}
	}
	caps := media.Caps{Video: []string{"h264"}, Audio: []string{"aac"}}
	enc := s.Encoders.PickFor(s.Store.Setting("encoder"), s.Store.Setting("video_codec"), caps)
	sess, err := s.Media.Start(s.Ctx, media.HLSSpec{Item: it, UserID: c.User.ID, Decision: ps.Decision, Start: ps.Start, Encoder: enc})
	if err != nil {
		return nil, playID, err
	}
	ps.Kinora = sess.ID
	return sess, playID, nil
}

func (s *Server) handleMaster(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.playable(r.PathValue("item"), c)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	_, playID, err := s.session(r, c, it)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, media.ErrBusy) {
			status = http.StatusServiceUnavailable
		}
		s.Log.Warn("jellyfin-compat: transcode failed", "item", it.Title, "err", err)
		writeErr(w, status, err.Error())
		return
	}
	bw := it.Bitrate
	if bw <= 0 {
		bw = 8_000_000
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=%d\nmain.m3u8?PlaySessionId=%s&api_key=%s\n", bw, playID, c.Token)
}

var reSegURI = regexp.MustCompile(`URI="([^"]+)"`)

func (s *Server) handleMain(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.playable(r.PathValue("item"), c)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	sess, _, err := s.session(r, c, it)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pl, err := sess.Playlist()
	if err != nil {
		writeErr(w, http.StatusNotFound, "the playlist is not ready yet")
		return
	}
	seg := func(name string) string { return "hls1/" + sess.ID + "/" + name + "?api_key=" + c.Token }
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(pl))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			line = reSegURI.ReplaceAllStringFunc(line, func(m string) string { return `URI="` + seg(reSegURI.FindStringSubmatch(m)[1]) + `"` })
		case line != "" && !strings.HasPrefix(line, "#"):
			line = seg(line)
		}
		out.WriteString(line + "\n")
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(out.Bytes())
}

var reHLSFile = regexp.MustCompile(`^(init\.mp4|seg\d{5,}\.(ts|m4s))$`)

func (s *Server) handleSegment(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	file := r.PathValue("file")
	if !reHLSFile.MatchString(file) {
		writeErr(w, http.StatusBadRequest, "bad segment name")
		return
	}
	sess := s.Media.Get(r.PathValue("session"), c.User.ID)
	if id, ok := s.ids.decode(nsItem, r.PathValue("item")); sess == nil || !ok || sess.ItemID != id {
		writeErr(w, http.StatusNotFound, "the session has ended")
		return
	}
	p := filepath.Join(sess.Dir, file)
	for i := 0; i < 100; i++ { // a player may ask for the next segment a moment before ffmpeg finishes it
		if _, err := os.Stat(p); err == nil || sess.Finished() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(p); err != nil {
		writeErr(w, http.StatusNotFound, "the segment is not ready")
		return
	}
	if strings.HasSuffix(file, ".ts") {
		w.Header().Set("Content-Type", "video/mp2t")
	} else {
		w.Header().Set("Content-Type", "video/mp4")
	}
	http.ServeFile(w, r, p)
}

func (s *Server) stopPlay(playID string, userID int64) {
	if ps := s.plays.remove(playID); ps != nil && ps.UserID == userID && ps.Kinora != "" {
		if sess := s.Media.Get(ps.Kinora, userID); sess != nil {
			s.Media.Stop(sess.ID)
		}
	}
}

func (s *Server) handleStopEncodings(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if id := qv(r, "PlaySessionId"); id != "" {
		s.stopPlay(id, c.User.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ------------------------------------------------------------------ subtitles

func (s *Server) handleSubtitle(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.playable(r.PathValue("item"), c)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	jf, err := strconv.Atoi(r.PathValue("index"))
	ti := typeIndex(it, jf, "subtitle")
	if err != nil || ti < 0 {
		writeErr(w, http.StatusNotFound, "no such subtitle")
		return
	}
	var st store.StreamInfo
	for _, x := range it.Streams {
		if x.Type == "subtitle" && x.TypeIndex == ti {
			st = x
		}
	}
	p, err := s.Subs.VTT(r.Context(), it, st)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(r.PathValue("file")), "."))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if format == "srt" || format == "subrip" {
		b, err := os.ReadFile(p)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "subtitle not readable")
			return
		}
		w.Header().Set("Content-Type", "application/x-subrip; charset=utf-8")
		w.Write(vttToSRT(b))
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	http.ServeFile(w, r, p)
}

var reVTTTime = regexp.MustCompile(`(\d{2}:)?(\d{2}):(\d{2})\.(\d{3})`)

// vttToSRT converts WebVTT to SubRip: numbered cues, comma decimals, no header or cue settings.
func vttToSRT(b []byte) []byte {
	var out bytes.Buffer
	blocks := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n\n")
	n := 0
	for _, blk := range blocks {
		lines := strings.Split(strings.TrimSpace(blk), "\n")
		i := 0
		for i < len(lines) && !strings.Contains(lines[i], "-->") {
			i++
		}
		if i >= len(lines) {
			continue
		}
		parts := strings.SplitN(lines[i], "-->", 2)
		end := strings.Fields(parts[1])
		if len(end) == 0 {
			continue
		}
		conv := func(t string) string {
			m := reVTTTime.FindStringSubmatch(strings.TrimSpace(t))
			if m == nil {
				return "00:00:00,000"
			}
			h := strings.TrimSuffix(m[1], ":")
			if h == "" {
				h = "00"
			}
			return h + ":" + m[2] + ":" + m[3] + "," + m[4]
		}
		n++
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n", n, conv(parts[0]), conv(end[0]), strings.Join(lines[i+1:], "\n"))
	}
	return out.Bytes()
}

// ------------------------------------------------------------------ playback reports and user data

type playReport struct {
	ItemID        string `json:"ItemId"`
	PositionTicks int64  `json:"PositionTicks"`
	PlaySessionID string `json:"PlaySessionId"`
	IsPaused      bool   `json:"IsPaused"`
	Failed        bool   `json:"Failed"`
}

func (s *Server) report(c *reqCtx, rep playReport, stopped bool) {
	it, ok := s.playable(rep.ItemID, c)
	if !ok {
		return
	}
	s.Media.TouchItem(c.User.ID, it.ID)
	pos := float64(rep.PositionTicks) / 1e7
	if pos > 0 || stopped {
		if _, err := s.Store.ReportProgress(c.Profile.ID, it, pos, false); err != nil {
			s.Log.Warn("jellyfin-compat: progress", "err", err)
		}
	}
	if stopped && rep.PlaySessionID != "" {
		s.stopPlay(rep.PlaySessionID, c.User.ID)
	}
}

func (s *Server) handlePlaying(stopped bool) handler {
	return func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		var rep playReport
		decode(r, &rep)
		s.report(c, rep, stopped)
		w.WriteHeader(http.StatusNoContent)
	}
}

// the older form: /Users/{user}/PlayingItems/{item}[/Progress]?PositionTicks=…
func (s *Server) handlePlayingLegacy(stopped bool) handler {
	return func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		t, _ := strconv.ParseInt(qv(r, "PositionTicks"), 10, 64)
		s.report(c, playReport{ItemID: r.PathValue("item"), PositionTicks: t, PlaySessionID: qv(r, "PlaySessionId")}, stopped)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) userDataDTO(it *store.Item, c *reqCtx) map[string]any {
	ud := s.Store.GetUserData(c.Profile.ID, it.ID)
	u := map[string]any{"PlaybackPositionTicks": ticks(ud.Position), "PlayCount": ud.PlayCount, "IsFavorite": ud.Favorite,
		"Played": ud.Played, "Key": s.itemID(it.ID), "ItemId": s.itemID(it.ID)}
	if ud.LastPlayedAt != nil {
		u["LastPlayedDate"] = isoTime(*ud.LastPlayedAt)
	}
	return u
}

func (s *Server) handlePlayed(played bool) handler {
	return func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		id, ok := s.ids.decode(nsItem, r.PathValue("item"))
		it, err := s.Store.Item(id, c.Profile)
		if !ok || err != nil {
			writeErr(w, http.StatusNotFound, "item not found")
			return
		}
		if err := s.Store.SetPlayed(c.Profile.ID, it, played); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not save")
			return
		}
		writeJSON(w, http.StatusOK, s.userDataDTO(it, c))
	}
}

func (s *Server) handleFavorite(fav bool) handler {
	return func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		id, ok := s.ids.decode(nsItem, r.PathValue("item"))
		it, err := s.Store.Item(id, c.Profile)
		if !ok || err != nil {
			writeErr(w, http.StatusNotFound, "item not found")
			return
		}
		if err := s.Store.SetFlag(c.Profile.ID, it.ID, "favorite", fav); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not save")
			return
		}
		writeJSON(w, http.StatusOK, s.userDataDTO(it, c))
	}
}

// ------------------------------------------------------------------ media segments (intro / credits)

func (s *Server) handleSegments(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.playable(r.PathValue("item"), c)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"Items": []any{}, "TotalRecordCount": 0, "StartIndex": 0})
		return
	}
	stored, _ := s.Store.StoredSegments(it.ID)
	jfType := map[string]string{"intro": "Intro", "credits": "Outro", "recap": "Recap", "preview": "Preview"}
	items := []map[string]any{}
	for i, g := range store.EffectiveSegments(stored, it.Chapters, float64(it.RuntimeSec)) {
		t := jfType[g.Type]
		if t == "" || g.End <= g.Start {
			continue
		}
		items = append(items, map[string]any{"Id": fmt.Sprintf("%s-%d", s.itemID(it.ID)[:24], i), "ItemId": s.itemID(it.ID), "Type": t,
			"StartTicks": ticks(g.Start), "EndTicks": ticks(g.End)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}
