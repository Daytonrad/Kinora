package server

import "testing"

func TestContentDispositionEscapesParamSeparators(t *testing.T) {
	got := contentDisposition("attachment", "Árvíz; 2. rész (1080p), 'x'=y.mkv")
	want := `attachment; filename="_rv_z; 2. r_sz (1080p), 'x'=y.mkv"; ` +
		`filename*=UTF-8''%C3%81rv%C3%ADz%3B%202.%20r%C3%A9sz%20%281080p%29%2C%20%27x%27%3Dy.mkv`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
