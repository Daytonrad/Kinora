package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// maxPlain keeps the encrypted payload under the relay's 2048-byte limit.
const maxPlain = 1500

// Message is the plaintext the iOS Notification Service Extension decrypts.
type Message struct {
	V     int    `json:"v"`
	Type  string `json:"type"` // request_available | test
	Title string `json:"title"`
	Body  string `json:"body"`
	Item  int64  `json:"item,omitempty"`  // library item to open
	Href  string `json:"href,omitempty"`  // the same in the web client
	Image string `json:"image,omitempty"` // path on this server, fetched with the app's session
	Tag   string `json:"tag,omitempty"`
}

// Notifier pushes fulfilled requests to their requester's paired iOS devices.
type Notifier struct {
	st       *store.Store
	Relay    *Relay
	log      *slog.Logger
	Interval time.Duration
}

// NewNotifier makes the notifier; Run drives it.
func NewNotifier(st *store.Store, relay *Relay, log *slog.Logger) *Notifier {
	return &Notifier{st: st, Relay: relay, log: log, Interval: 30 * time.Second}
}

// Run sends pending notifications until ctx ends.
func (n *Notifier) Run(ctx context.Context) {
	t := time.NewTicker(n.Interval)
	defer t.Stop()
	for {
		n.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick sends what is pending now. Requests fulfilled more than a day ago (push was
// off) are marked as done without sending.
func (n *Notifier) Tick(ctx context.Context) {
	if !n.Relay.Enabled() {
		return
	}
	list, err := n.st.PendingAvailable(time.Now().Add(-24 * time.Hour).Unix())
	if err != nil {
		n.log.Warn("push: pending requests", "err", err)
		return
	}
	for _, q := range list {
		if ctx.Err() != nil {
			return
		}
		m := Message{V: 1, Type: "request_available", Title: q.Title, Body: availableBody(q), Tag: fmt.Sprintf("req-%d", q.ID)}
		if id := n.st.LibraryItemByTmdb(q.Kind, q.TmdbID); id > 0 {
			m.Item, m.Href, m.Image = id, "#/item/"+strconv.FormatInt(id, 10), fmt.Sprintf("/api/images/%d/poster", id)
		}
		if err := n.SendUser(ctx, *q.UserID, m); err != nil && !errors.Is(err, ErrDisabled) {
			n.log.Warn("push: request", "id", q.ID, "err", err)
			var re *RelayError
			if !errors.As(err, &re) || re.Status >= 500 || re.Status == 429 {
				return // relay unreachable or busy: retry the rest on the next tick
			}
		}
		n.st.MarkPushed(q.ID)
	}
}

func availableBody(q *store.AcqRequest) string {
	if q.Kind == "series" {
		return "A kért sorozat megérkezett, már nézheted."
	}
	return "A kért film megérkezett, már nézheted."
}

// SendUser pushes m to every paired device of an account. Dead handles are dropped.
// It returns the first error that was not a dead handle.
func (n *Notifier) SendUser(ctx context.Context, userID int64, m Message) error {
	devs, err := n.st.PushIOSForUser(userID)
	if err != nil {
		return err
	}
	var first error
	for _, d := range devs {
		if err := n.SendDevice(ctx, d, m); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// SendDevice pushes m to one paired device.
func (n *Notifier) SendDevice(ctx context.Context, d store.PushIOS, m Message) error {
	plain, err := encode(m)
	if err != nil {
		return err
	}
	sealed, err := Seal(d.Key, plain)
	if err != nil {
		return err
	}
	err = n.Relay.Send(ctx, d.Handle, sealed, m.Tag)
	if errors.Is(err, ErrGone) {
		n.st.DeletePushIOSHandle(d.Handle)
		return nil
	}
	var re *RelayError
	if errors.As(err, &re) && re.Code == "unknown_handle" {
		n.st.DeletePushIOSHandle(d.Handle)
		return nil
	}
	return err
}

// encode marshals m, shortening the body (then the title) until it fits.
func encode(m Message) ([]byte, error) {
	m.Title, m.Body = clip(m.Title, 150), clip(m.Body, 400)
	for range 16 {
		b, err := json.Marshal(m)
		if err != nil || len(b) <= maxPlain {
			return b, err
		}
		if r := []rune(m.Body); len(r) > 40 {
			m.Body = string(r[:len(r)*2/3]) + "…"
		} else if r := []rune(m.Title); len(r) > 40 {
			m.Title = string(r[:len(r)*2/3]) + "…"
		} else {
			m.Image, m.Href = "", ""
		}
	}
	return nil, errors.New("push: az üzenet túl hosszú")
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
