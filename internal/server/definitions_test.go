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
	// the built-in ones (nCore) come first and cannot be deleted or replaced
	if c.do("GET", "/api/admin/acquire/definitions", nil, &list); len(list) != 2 || list[0]["id"] != "ncore" || list[0]["builtin"] != true || list[1]["name"] != "Demo Site" {
		t.Fatalf("list: %v", list)
	}
	if code := c.do("DELETE", "/api/admin/acquire/definitions/ncore", nil, nil); code != 409 {
		t.Errorf("delete built-in: %d", code)
	}
	if code := c.do("POST", "/api/admin/acquire/definitions", map[string]any{"yaml": strings.Replace(testDefinition, "id: demo", "id: ncore", 1)}, nil); code != 409 {
		t.Errorf("replace built-in: %d", code)
	}
	var nc map[string]any
	if code := c.do("POST", "/api/admin/acquire/indexers", map[string]any{"kind": "cardigann", "definition_id": "ncore", "enabled": true,
		"config": `{"username":"anna","password":"pw","cookie":""}`}, &nc); code != 200 || nc["name"] != "nCore" || !strings.Contains(nc["config"].(string), `"password":"••••••••"`) {
		t.Errorf("nCore indexer: %d %v", code, nc)
	}
	c.do("DELETE", fmt.Sprintf("/api/admin/acquire/indexers/%d", int64(nc["id"].(float64))), nil, nil)
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
