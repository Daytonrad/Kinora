package server

import (
	"fmt"
	"strings"
	"testing"
)

const testDefinition = `id: demo
name: Demo Site
type: private
links: [http://127.0.0.1:1/]
settings:
  - {name: username, type: text, label: User}
  - {name: password, type: password, label: Password}
  - {name: note, type: info, label: Just text}
search:
  paths: [{path: browse}]
  rows: {selector: tr}
  fields:
    title: {selector: a}
    download: {selector: a, attribute: href}
`

func TestAcqDefinitionsAPI(t *testing.T) {
	c, st := newTestServer(t)
	if code := c.do("POST", "/api/admin/acquire/definitions", map[string]any{"yaml": "name: no id"}, nil); code != 400 {
		t.Errorf("invalid definition: %d", code)
	}
	var info map[string]any
	if code := c.do("POST", "/api/admin/acquire/definitions", map[string]any{"yaml": testDefinition}, &info); code != 200 || info["id"] != "demo" || len(info["settings"].([]any)) != 3 {
		t.Fatalf("upload: %d %v", code, info)
	}
	var list []map[string]any
	if c.do("GET", "/api/admin/acquire/definitions", nil, &list); len(list) != 1 || list[0]["name"] != "Demo Site" {
		t.Fatalf("list: %v", list)
	}
	if code := c.do("POST", "/api/admin/acquire/indexers", map[string]any{"kind": "cardigann", "definition_id": "nope", "enabled": true}, nil); code != 400 {
		t.Errorf("unknown definition: %d", code)
	}
	var x map[string]any
	if code := c.do("POST", "/api/admin/acquire/indexers", map[string]any{"kind": "cardigann", "definition_id": "demo", "enabled": true,
		"config": `{"username":"anna","password":"supersecret","note":"x","extra":"y"}`}, &x); code != 200 {
		t.Fatalf("indexer: %d %v", code, x)
	}
	if x["name"] != "Demo Site" || strings.Contains(x["config"].(string), "supersecret") || !strings.Contains(x["config"].(string), "anna") {
		t.Errorf("saved indexer: %v", x)
	}
	id := int64(x["id"].(float64))
	// saved again with the masked password: the stored one stays; unknown and info values are dropped
	x["config"] = strings.Replace(x["config"].(string), "anna", "bela", 1)
	if code := c.do("PATCH", fmt.Sprintf("/api/admin/acquire/indexers/%d", id), x, nil); code != 200 {
		t.Fatalf("update: %d", code)
	}
	stored, _ := st.AcqIndexer(id)
	if stored.Config != `{"password":"supersecret","username":"bela"}` || stored.URL != "http://127.0.0.1:1/" {
		t.Errorf("stored: %+v", stored)
	}
	if code := c.do("DELETE", "/api/admin/acquire/definitions/demo", nil, nil); code != 409 {
		t.Errorf("delete while in use: %d", code)
	}
	c.do("DELETE", fmt.Sprintf("/api/admin/acquire/indexers/%d", id), nil, nil)
	if code := c.do("DELETE", "/api/admin/acquire/definitions/demo", nil, nil); code != 200 {
		t.Errorf("delete: %d", code)
	}
}
