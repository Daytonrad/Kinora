package media

import (
	"testing"
	"time"
)

// A position report keeps the reporting user's sessions of that item alive.
func TestTouchItem(t *testing.T) {
	m := NewManager(t.TempDir(), nil, quiet)
	old := time.Now().Add(-time.Hour).UnixNano()
	add := func(id string, user, item int64) *Session {
		s := &Session{ID: id, UserID: user, ItemID: item, done: make(chan struct{})}
		s.lastSeen.Store(old)
		m.sess[id] = s
		return s
	}
	mine, otherItem, otherUser := add("a", 1, 10), add("b", 1, 11), add("c", 2, 10)
	m.TouchItem(1, 10)
	if mine.lastSeen.Load() == old {
		t.Fatal("the reported item's session was not touched")
	}
	if otherItem.lastSeen.Load() != old || otherUser.lastSeen.Load() != old {
		t.Fatal("other sessions must not be touched")
	}
	var nilManager *Manager
	nilManager.TouchItem(1, 10)
}
