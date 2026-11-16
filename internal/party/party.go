// Package party implements watch parties over WebSocket.
//
// Jellyfin problem it addresses: SyncPlay stutters and often needs a rejoin
// after a pause. Here the server owns the clock: the room state is
// (pos, ref time, intended), actual playback = intended AND nobody buffering,
// and clients sync to it with an NTP-like offset, nudging playbackRate for
// small drift and seeking only for large drift. A buffering member pauses the
// room, but only for up to 12 s – then the others continue without them.
package party

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Tunables (variables so tests can shorten them).
var (
	BufferTimeout = 12 * time.Second
	EmptyTTL      = 10 * time.Minute
	PingInterval  = 25 * time.Second
	ReadDeadline  = 70 * time.Second
)

// Reactions is the allowed emoji list.
var Reactions = []string{"😂", "😮", "😍", "👏", "😱", "🔥", "👍", "😢"}

// Who identifies a joining viewer.
type Who struct {
	UserID    int64
	ProfileID int64
	Name      string
	Color     string
}

// ChatMsg is a chat line.
type ChatMsg struct {
	T     string `json:"t"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Text  string `json:"text"`
	At    int64  `json:"at"`
}

type member struct {
	id        int
	who       Who
	conn      *websocket.Conn
	send      chan []byte
	buffering bool
	bufSince  time.Time
	ignored   bool // buffered too long: the room continues without them
	host      bool
	closed    atomic.Bool
}

// Room is one party.
type Room struct {
	Code    string
	OwnerID int64

	mu         sync.Mutex
	itemID     int64
	everyone   bool
	pos        float64
	ref        time.Time
	intended   bool
	playing    bool
	by         int
	seq        int
	members    []*member
	nextID     int
	chat       []ChatMsg
	emptySince time.Time
	hub        *Hub
}

// Hub owns all rooms.
type Hub struct {
	log *slog.Logger
	// CanSee reports whether a profile may see an item (rating filter).
	CanSee func(profileID, itemID int64) bool

	mu    sync.Mutex
	rooms map[string]*Room
	up    websocket.Upgrader
}

// NewHub creates a hub.
func NewHub(log *slog.Logger) *Hub {
	h := &Hub{log: log, rooms: map[string]*Room{}, CanSee: func(int64, int64) bool { return true }}
	h.up = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: sameOrigin}
	return h
}

// sameOrigin accepts native clients (no Origin) and same-host browsers.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return strings.EqualFold(u.Host, host)
}

// Create makes a new room.
func (h *Hub) Create(itemID int64, everyone bool, owner int64) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	var code string
	for {
		code = store.RandomCode(6)
		if h.rooms[code] == nil {
			break
		}
	}
	r := &Room{Code: code, OwnerID: owner, itemID: itemID, everyone: everyone, ref: time.Now(), emptySince: time.Now(), hub: h}
	h.rooms[code] = r
	h.log.Info("party created", "code", code, "item", itemID)
	return r
}

// Get finds a room by code (case-insensitive).
func (h *Hub) Get(code string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rooms[strings.ToUpper(strings.TrimSpace(code))]
}

// Count returns the number of rooms and connected members.
func (h *Hub) Count() (rooms, members int) {
	h.mu.Lock()
	rs := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rs = append(rs, r)
	}
	h.mu.Unlock()
	for _, r := range rs {
		r.mu.Lock()
		members += len(r.members)
		r.mu.Unlock()
	}
	return len(rs), members
}

// Info is a public room summary.
type Info struct {
	Code     string `json:"code"`
	ItemID   int64  `json:"item_id"`
	Members  int    `json:"members"`
	Everyone bool   `json:"everyone"`
	Playing  bool   `json:"playing"`
}

// Info returns a summary.
func (r *Room) Info() Info {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Info{Code: r.Code, ItemID: r.itemID, Members: len(r.members), Everyone: r.everyone, Playing: r.playing}
}

// Run reaps empty rooms and enforces the buffering timeout.
func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	lastReap := time.Now()
	for {
		select {
		case <-ctx.Done():
			h.closeAll()
			return
		case <-t.C:
			h.mu.Lock()
			rs := make([]*Room, 0, len(h.rooms))
			for _, r := range h.rooms {
				rs = append(rs, r)
			}
			h.mu.Unlock()
			for _, r := range rs {
				r.tick()
			}
			if time.Since(lastReap) > 30*time.Second || EmptyTTL < time.Minute {
				lastReap = time.Now()
				h.reap()
			}
		}
	}
}

func (h *Hub) reap() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for code, r := range h.rooms {
		r.mu.Lock()
		empty := len(r.members) == 0 && time.Since(r.emptySince) > EmptyTTL
		r.mu.Unlock()
		if empty {
			delete(h.rooms, code)
			h.log.Info("party closed (empty)", "code", code)
		}
	}
}

func (h *Hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.rooms {
		r.mu.Lock()
		for _, m := range r.members {
			m.conn.Close()
		}
		r.mu.Unlock()
	}
}

// ------------------------------------------------------------------ room state (r.mu held)

func (r *Room) current(now time.Time) float64 {
	if r.playing {
		return r.pos + now.Sub(r.ref).Seconds()
	}
	return r.pos
}

func (r *Room) waiting() []string {
	var w []string
	for _, m := range r.members {
		if m.buffering && !m.ignored {
			w = append(w, m.who.Name)
		}
	}
	return w
}

// recompute updates `playing` from intent and buffering, freezing/unfreezing the clock.
func (r *Room) recompute(now time.Time) {
	should := r.intended && len(r.waiting()) == 0
	if should == r.playing {
		return
	}
	if r.playing {
		r.pos = r.current(now)
	}
	r.ref = now
	r.playing = should
}

type stateMsg struct {
	T        string        `json:"t"`
	Code     string        `json:"code"`
	ItemID   int64         `json:"item_id"`
	Playing  bool          `json:"playing"`
	Intended bool          `json:"intended"`
	Pos      float64       `json:"pos"`
	At       int64         `json:"at"` // server ms when pos was sampled
	Waiting  []string      `json:"waiting"`
	Members  []memberState `json:"members"`
	Everyone bool          `json:"everyone"`
	By       int           `json:"by"`
	Seq      int           `json:"seq"`
}

type memberState struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Buffering bool   `json:"buffering"`
	Host      bool   `json:"host"`
}

func (r *Room) stateLocked() []byte {
	now := time.Now()
	st := stateMsg{T: "state", Code: r.Code, ItemID: r.itemID, Playing: r.playing, Intended: r.intended,
		Pos: r.current(now), At: now.UnixMilli(), Waiting: r.waiting(), Everyone: r.everyone, By: r.by, Seq: r.seq}
	if st.Waiting == nil {
		st.Waiting = []string{}
	}
	for _, m := range r.members {
		st.Members = append(st.Members, memberState{m.id, m.who.Name, m.who.Color, m.buffering && !m.ignored, m.host})
	}
	b, _ := json.Marshal(st)
	return b
}

func (r *Room) broadcastLocked(b []byte) {
	for _, m := range r.members {
		m.push(b)
	}
}

// push never blocks: a slow client just misses messages (the next state catches it up).
func (m *member) push(b []byte) {
	if m.closed.Load() {
		return
	}
	select {
	case m.send <- b:
	default:
	}
}

func (r *Room) sysLocked(text string) {
	b, _ := json.Marshal(map[string]any{"t": "sys", "text": text, "at": time.Now().UnixMilli()})
	r.broadcastLocked(b)
}

func (r *Room) changed(by *member) {
	r.seq++
	r.by = 0
	if by != nil {
		r.by = by.id
	}
	r.broadcastLocked(r.stateLocked())
}

func (r *Room) tick() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, m := range r.members {
		if m.buffering && !m.ignored && now.Sub(m.bufSince) > BufferTimeout {
			m.ignored = true
			r.sysLocked(fmt.Sprintf("%s túl sokáig töltött – nélküle folytatjuk", m.who.Name))
			r.recompute(now)
			r.changed(nil)
		}
	}
}

func fmtTime(sec float64) string {
	s := int(sec)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// ------------------------------------------------------------------ connections

type inMsg struct {
	T      string   `json:"t"`
	C      float64  `json:"c"`
	Pos    *float64 `json:"pos"`
	V      *bool    `json:"v"`
	ItemID int64    `json:"item_id"`
	Text   string   `json:"text"`
	Emoji  string   `json:"emoji"`
}

// Serve upgrades the request and runs the member until it disconnects.
func (h *Hub) Serve(w http.ResponseWriter, req *http.Request, r *Room, who Who) {
	conn, err := h.up.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	m := &member{who: who, conn: conn, send: make(chan []byte, 64)}

	r.mu.Lock()
	r.nextID++
	m.id = r.nextID
	if len(r.members) == 0 || !hasHost(r.members) {
		m.host = true
	}
	r.members = append(r.members, m)
	hello, _ := json.Marshal(map[string]any{"t": "hello", "id": m.id, "reactions": Reactions})
	m.push(hello)
	for _, c := range r.chat {
		b, _ := json.Marshal(c)
		m.push(b)
	}
	r.sysLocked(who.Name + " csatlakozott")
	r.changed(nil)
	r.mu.Unlock()
	h.log.Info("party join", "code", r.Code, "name", who.Name)

	done := make(chan struct{})
	go m.writer(done)
	h.reader(r, m)
	close(done)

	r.mu.Lock()
	m.closed.Store(true)
	for i, x := range r.members {
		if x == m {
			r.members = append(r.members[:i], r.members[i+1:]...)
			break
		}
	}
	if m.host && len(r.members) > 0 {
		r.members[0].host = true
		r.sysLocked(r.members[0].who.Name + " lett a házigazda")
	}
	if len(r.members) == 0 {
		r.emptySince = time.Now()
	}
	r.sysLocked(who.Name + " kilépett")
	r.recompute(time.Now())
	r.changed(nil)
	r.mu.Unlock()
	conn.Close()
}

func hasHost(ms []*member) bool {
	for _, m := range ms {
		if m.host {
			return true
		}
	}
	return false
}

func (m *member) writer(done chan struct{}) {
	t := time.NewTicker(PingInterval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case b := <-m.send:
			m.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := m.conn.WriteMessage(websocket.TextMessage, b); err != nil {
				m.conn.Close()
				return
			}
		case <-t.C:
			m.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := m.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				m.conn.Close()
				return
			}
		}
	}
}

func (h *Hub) reader(r *Room, m *member) {
	m.conn.SetReadLimit(8192)
	m.conn.SetReadDeadline(time.Now().Add(ReadDeadline))
	m.conn.SetPongHandler(func(string) error { m.conn.SetReadDeadline(time.Now().Add(ReadDeadline)); return nil })
	for {
		_, data, err := m.conn.ReadMessage()
		if err != nil {
			return
		}
		m.conn.SetReadDeadline(time.Now().Add(ReadDeadline))
		var in inMsg
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		h.handle(r, m, in)
	}
}

func (h *Hub) handle(r *Room, m *member, in inMsg) {
	now := time.Now()
	if in.T == "ping" {
		b, _ := json.Marshal(map[string]any{"t": "pong", "c": in.C, "s": now.UnixMilli()})
		m.push(b)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	control := m.host || r.everyone
	deny := func() {
		b, _ := json.Marshal(map[string]any{"t": "sys", "text": "csak a házigazda irányíthat", "at": now.UnixMilli()})
		m.push(b)
		m.push(r.stateLocked()) // snap the client back
	}
	switch in.T {
	case "play":
		if !control {
			deny()
			return
		}
		if !r.intended {
			r.intended = true
			r.recompute(now)
			r.sysLocked(m.who.Name + " elindította")
			r.changed(m)
		}
	case "pause":
		if !control {
			deny()
			return
		}
		if in.Pos != nil {
			r.pos = clampPos(*in.Pos)
		} else {
			r.pos = r.current(now)
		}
		r.ref = now
		r.intended = false
		r.playing = false
		r.sysLocked(m.who.Name + " megállította")
		r.changed(m)
	case "seek":
		if !control || in.Pos == nil {
			deny()
			return
		}
		r.pos = clampPos(*in.Pos)
		r.ref = now
		r.sysLocked(fmt.Sprintf("%s ide tekert: %s", m.who.Name, fmtTime(r.pos)))
		r.changed(m)
	case "item":
		if !control || in.ItemID == 0 {
			deny()
			return
		}
		for _, x := range r.members {
			if !h.CanSee(x.who.ProfileID, in.ItemID) {
				b, _ := json.Marshal(map[string]any{"t": "sys", "text": "ezt a címet nem mindenki láthatja a szobában", "at": now.UnixMilli()})
				m.push(b)
				return
			}
		}
		r.itemID = in.ItemID
		r.pos = 0
		if in.Pos != nil {
			r.pos = clampPos(*in.Pos)
		}
		r.ref = now
		r.sysLocked(m.who.Name + " másik részt indított")
		r.changed(m)
	case "buffering":
		v := in.V != nil && *in.V
		if v == m.buffering {
			return
		}
		m.buffering = v
		if v {
			m.bufSince = now
		} else {
			m.ignored = false
		}
		r.recompute(now)
		r.changed(nil)
	case "everyone":
		if !m.host || in.V == nil {
			deny()
			return
		}
		r.everyone = *in.V
		if r.everyone {
			r.sysLocked("mostantól mindenki irányíthat")
		} else {
			r.sysLocked("mostantól csak a házigazda irányíthat")
		}
		r.changed(m)
	case "chat":
		text := strings.TrimSpace(in.Text)
		if text == "" {
			return
		}
		if utf8.RuneCountInString(text) > 500 {
			text = string([]rune(text)[:500])
		}
		c := ChatMsg{T: "chat", Name: m.who.Name, Color: m.who.Color, Text: text, At: now.UnixMilli()}
		r.chat = append(r.chat, c)
		if len(r.chat) > 50 {
			r.chat = r.chat[len(r.chat)-50:]
		}
		b, _ := json.Marshal(c)
		r.broadcastLocked(b)
	case "react":
		for _, e := range Reactions {
			if e == in.Emoji {
				b, _ := json.Marshal(map[string]any{"t": "react", "name": m.who.Name, "color": m.who.Color, "emoji": e})
				r.broadcastLocked(b)
				return
			}
		}
	}
}

func clampPos(p float64) float64 {
	if p < 0 {
		return 0
	}
	return p
}
