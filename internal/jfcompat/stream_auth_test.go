package jfcompat

import "testing"

// TestStreamByPlaySession: JellyCon (Kodi) gives its player the stream URL without a token, only with the
// PlaySessionId of PlaybackInfo. That id works for its own item and only while the token behind it is valid.
func TestStreamByPlaySession(t *testing.T) {
	e := newEnv(t)
	tok := e.login("anna", "secret123")
	var films itemList
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie&SortBy=SortName", tok, nil, &films)
	big, space := films.find("Big Test Movie"), films.find("Space Drama")
	var pi struct{ PlaySessionId string }
	if code, b := e.call("POST", "/Items/"+big+"/PlaybackInfo", tok, map[string]any{}, &pi); code != 200 || pi.PlaySessionId == "" {
		t.Fatalf("PlaybackInfo: %d %s", code, b)
	}
	url := func(item, play string) string {
		return "/Videos/" + item + "/stream?static=True&PlaySessionId=" + play + "&MediaSourceId=" + item
	}
	if code, _ := e.call("HEAD", url(big, pi.PlaySessionId), "", nil, nil); code != 200 {
		t.Fatalf("HEAD with the play session: %d", code)
	}
	if code, b := e.call("GET", url(big, pi.PlaySessionId), "", nil, nil); code != 200 || len(b) == 0 {
		t.Fatalf("GET with the play session: %d", code)
	}
	for name, u := range map[string]string{
		"no play session":         "/Videos/" + big + "/stream?static=True",
		"guessed play session":    url(big, "0123456789abcdef0123456789abcdef"),
		"another item":            url(space, pi.PlaySessionId),
		"another file of another": "/Videos/" + space + "/master.m3u8?PlaySessionId=" + pi.PlaySessionId,
	} {
		if code, _ := e.call("GET", u, "", nil, nil); code != 401 {
			t.Fatalf("%s: %d", name, code)
		}
	}
	// the kids profile's own play session does not open a film it cannot see (PlaybackInfo refuses it)
	kidTok := e.login("anna/Kicsi", "secret123#1234")
	var kpi struct{ PlaySessionId string }
	if code, _ := e.call("POST", "/Items/"+space+"/PlaybackInfo", kidTok, map[string]any{}, &kpi); code != 404 || kpi.PlaySessionId != "" {
		t.Fatalf("kids PlaybackInfo of an R film: %d", code)
	}
	// signing out ends the play session's access
	if code, _ := e.call("POST", "/Sessions/Logout", tok, nil, nil); code/100 != 2 {
		t.Fatalf("logout: %d", code)
	}
	if code, _ := e.call("GET", url(big, pi.PlaySessionId), "", nil, nil); code != 401 {
		t.Fatalf("after logout: %d", code)
	}
}
