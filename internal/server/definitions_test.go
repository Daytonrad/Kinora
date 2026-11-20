package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// the catalogue: a .tar.gz shaped like the Prowlarr repository, served locally
func TestAcqCatalogAPI(t *testing.T) {
	def := func(id, name, typ, ver string) string {
		return "id: " + id + "\nname: " + name + "\ntype: " + typ + "\nlanguage: en-US\ndescription: " + ver + "\nlinks: [http://127.0.0.1:1/]\n" +
			"search:\n  paths: [{path: browse}]\n  rows: {selector: tr}\n  fields:\n    title: {selector: a}\n    download: {selector: a, attribute: href}\n"
	}
	files := map[string]string{"Indexers-master/definitions/v10/old.yml": def("old", "Old", "public", "v10")}
	for i := 0; i < 25; i++ { // the newest well-filled version folder wins
		files[fmt.Sprintf("Indexers-master/definitions/v11/site%02d.yml", i)] = def(fmt.Sprintf("site%02d", i), fmt.Sprintf("Site %02d", i), "public", "one")
	}
	files["Indexers-master/definitions/v11/ncore.yml"] = def("ncore", "nCore", "private", "x") // Kinora has its own
	files["Indexers-master/definitions/v11/broken.yml"] = "id: [nope"
	version := "one"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for name, body := range files {
			if strings.HasSuffix(name, "site03.yml") {
				body = def("site03", "Site 03", "public", version)
			}
			tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
			tw.Write([]byte(body))
		}
		tw.Close()
		gz.Close()
		w.Write(buf.Bytes())
	}))
	defer srv.Close()
	t.Setenv("KINORA_INDEXER_CATALOG", srv.URL)
	c, _ := newTestServer(t)

	var list []map[string]any
	if code := c.do("GET", "/api/admin/acquire/catalog", nil, &list); code != 200 || len(list) != 25 || list[0]["id"] != "site00" || list[0]["installed"] != false {
		t.Fatalf("catalog: %d %d %v", code, len(list), list)
	}
	var info map[string]any
	if code := c.do("POST", "/api/admin/acquire/catalog/site03", nil, &info); code != 200 || info["id"] != "site03" {
		t.Fatalf("install: %d %v", code, info)
	}
	if code := c.do("POST", "/api/admin/acquire/catalog/nope", nil, nil); code != 404 {
		t.Errorf("unknown: %d", code)
	}
	// the catalogue moves on: the saved one shows as outdated and the update brings it along
	version = "two"
	c.do("GET", "/api/admin/acquire/catalog?refresh=1", nil, &list)
	if list[3]["installed"] != true || list[3]["outdated"] != true {
		t.Errorf("outdated: %v", list[3])
	}
	var up map[string]int
	if c.do("POST", "/api/admin/acquire/catalog/update", nil, &up); up["updated"] != 1 {
		t.Errorf("update: %v", up)
	}
	var defs []map[string]any
	c.do("GET", "/api/admin/acquire/definitions", nil, &defs)
	if len(defs) != 2 || defs[1]["description"] != "two" {
		t.Errorf("definitions after update: %v", defs)
	}

	// FlareSolverr: saved in the settings, tested by address
	var set map[string]any
	if code := c.do("PUT", "/api/admin/acquire/settings", map[string]any{"flaresolverr": "flaresolverr:8191"}, nil); code != 400 {
		t.Errorf("bad flaresolverr address: %d", code)
	}
	if c.do("PUT", "/api/admin/acquire/settings", map[string]any{"flaresolverr": "http://flaresolverr:8191/"}, &set); set["flaresolverr"] != "http://flaresolverr:8191" {
		t.Errorf("flaresolverr: %v", set["flaresolverr"])
	}
	if code := c.do("POST", "/api/admin/acquire/flaresolverr/test", map[string]any{"url": srv.URL}, nil); code != 502 {
		t.Errorf("not a FlareSolverr: %d", code)
	}
}
