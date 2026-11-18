package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// Collection is a manual or rule-based (smart) list. ProfileID nil = shared by
// the whole household (account).
type Collection struct {
	ID        int64    `json:"id"`
	ProfileID *int64   `json:"profile_id"`
	Shared    bool     `json:"shared"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // manual | smart
	Rules     *RuleSet `json:"rules,omitempty"`
	Pinned    bool     `json:"pinned"`
	CreatedAt int64    `json:"created_at"`
	Count     int      `json:"count"`
	userID    int64
}

const collCols = `id,profile_id,owner_user_id,name,kind,rules,pinned,created_at`

func scanCollection(row interface{ Scan(...any) error }) (*Collection, error) {
	c := &Collection{}
	var pid, uid sql.NullInt64
	var rules string
	if err := row.Scan(&c.ID, &pid, &uid, &c.Name, &c.Kind, &rules, &c.Pinned, &c.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if pid.Valid {
		c.ProfileID = &pid.Int64
	}
	c.Shared = !pid.Valid
	c.userID = uid.Int64
	if c.Kind == "smart" && rules != "" {
		var rs RuleSet
		if json.Unmarshal([]byte(rules), &rs) == nil {
			c.Rules = &rs
		}
	}
	return c, nil
}

// visibleColl restricts collections to the profile's own plus the household's shared ones.
const visibleColl = ` owner_user_id=? AND (profile_id IS NULL OR profile_id=?)`

// Collections lists the collections a profile can see (with item counts).
func (s *Store) Collections(p *Profile) ([]*Collection, error) {
	rows, err := s.DB.Query(`SELECT `+collCols+` FROM collections WHERE`+visibleColl+` ORDER BY pinned DESC, name COLLATE NOCASE`, p.UserID, p.ID)
	if err != nil {
		return nil, err
	}
	var out []*Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	for _, c := range out {
		_, c.Count, _ = s.CollectionItems(p, c, 0, 1)
	}
	if out == nil {
		out = []*Collection{}
	}
	return out, nil
}

// Collection loads one visible collection.
func (s *Store) Collection(p *Profile, id int64) (*Collection, error) {
	return scanCollection(s.DB.QueryRow(`SELECT `+collCols+` FROM collections WHERE id=? AND`+visibleColl, id, p.UserID, p.ID))
}

// CollectionInput is the editable part of a collection.
type CollectionInput struct {
	Name   *string  `json:"name"`
	Kind   string   `json:"kind"`
	Rules  *RuleSet `json:"rules"`
	Pinned *bool    `json:"pinned"`
	Shared *bool    `json:"shared"`
}

func (in *CollectionInput) validate() error {
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len([]rune(n)) > 80 {
			return errors.New("a gyűjtemény neve 1–80 karakter lehet")
		}
		in.Name = &n
	}
	if in.Rules != nil {
		b, _ := json.Marshal(in.Rules)
		rs, err := ParseRuleSet(string(b))
		if err != nil {
			return err
		}
		in.Rules = &rs
	}
	return nil
}

// CreateCollection adds a collection.
func (s *Store) CreateCollection(p *Profile, in CollectionInput) (*Collection, error) {
	if in.Kind != "smart" {
		in.Kind = "manual"
	}
	if in.Name == nil {
		return nil, errors.New("a gyűjtemény neve kötelező")
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	if in.Kind == "smart" && in.Rules == nil {
		in.Rules = &RuleSet{Match: "all"}
	}
	var pid any = p.ID
	if in.Shared != nil && *in.Shared {
		pid = nil
	}
	rules := ""
	if in.Rules != nil && in.Kind == "smart" {
		b, _ := json.Marshal(in.Rules)
		rules = string(b)
	}
	res, err := s.DB.Exec(`INSERT INTO collections(profile_id,owner_user_id,name,kind,rules,pinned,created_at) VALUES(?,?,?,?,?,?,?)`,
		pid, p.UserID, *in.Name, in.Kind, rules, in.Pinned != nil && *in.Pinned, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.Collection(p, id)
}

// UpdateCollection edits name/rules/pinned/shared.
func (s *Store) UpdateCollection(p *Profile, c *Collection, in CollectionInput) error {
	if err := in.validate(); err != nil {
		return err
	}
	set := []string{}
	args := []any{}
	if in.Name != nil {
		set, args = append(set, "name=?"), append(args, *in.Name)
	}
	if in.Rules != nil && c.Kind == "smart" {
		b, _ := json.Marshal(in.Rules)
		set, args = append(set, "rules=?"), append(args, string(b))
	}
	if in.Pinned != nil {
		set, args = append(set, "pinned=?"), append(args, *in.Pinned)
	}
	if in.Shared != nil {
		if *in.Shared {
			set = append(set, "profile_id=NULL")
		} else {
			set, args = append(set, "profile_id=?"), append(args, p.ID)
		}
	}
	if len(set) == 0 {
		return nil
	}
	_, err := s.DB.Exec(`UPDATE collections SET `+strings.Join(set, ",")+` WHERE id=?`, append(args, c.ID)...)
	return err
}

// DeleteCollection removes a collection.
func (s *Store) DeleteCollection(c *Collection) error {
	_, err := s.DB.Exec(`DELETE FROM collections WHERE id=?`, c.ID)
	return err
}

// CollectionItems returns a page of the collection's items and the total.
func (s *Store) CollectionItems(p *Profile, c *Collection, offset, limit int) ([]*Item, int, error) {
	if c.Kind == "smart" {
		rs := RuleSet{Match: "all"}
		if c.Rules != nil {
			rs = *c.Rules
		}
		return s.Browse(p, rs, offset, limit)
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	where := ` AND i.missing=0 AND EXISTS(SELECT 1 FROM collection_items ci WHERE ci.item_id=i.id AND ci.collection_id=?)`
	rf, rargs := RatingSQL(p, "i")
	var total int
	s.DB.QueryRow(`SELECT COUNT(*) FROM items i WHERE 1=1`+where+rf, append([]any{c.ID}, rargs...)...).Scan(&total)
	items, err := s.QueryItems(p, where, []any{c.ID},
		`ORDER BY (SELECT ci.position FROM collection_items ci WHERE ci.item_id=i.id AND ci.collection_id=?), `+locSort+` LIMIT ? OFFSET ?`,
		c.ID, limit, offset)
	if err == nil {
		s.fillGenresTags(items, p)
	}
	return items, total, err
}

// AddToCollection appends an item to a manual collection.
func (s *Store) AddToCollection(c *Collection, itemID int64) error {
	if c.Kind != "manual" {
		return errors.New("smart gyűjteménybe nem lehet kézzel elemet tenni")
	}
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO collection_items(collection_id,item_id,position)
		VALUES(?,?,(SELECT COALESCE(MAX(position),0)+1 FROM collection_items WHERE collection_id=?))`, c.ID, itemID, c.ID)
	return err
}

// RemoveFromCollection removes an item from a manual collection.
func (s *Store) RemoveFromCollection(c *Collection, itemID int64) error {
	_, err := s.DB.Exec(`DELETE FROM collection_items WHERE collection_id=? AND item_id=?`, c.ID, itemID)
	return err
}

// ReorderCollection sets the manual order.
func (s *Store) ReorderCollection(c *Collection, ids []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE collection_items SET position=? WHERE collection_id=? AND item_id=?`, i+1, c.ID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CollectionRef is a lightweight collection reference.
type CollectionRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// CollectionsOf lists visible collections that contain an item (manual
// membership or matching smart rules).
func (s *Store) CollectionsOf(p *Profile, itemID int64) []CollectionRef {
	out := []CollectionRef{}
	cs, err := s.Collections(p)
	if err != nil {
		return out
	}
	for _, c := range cs {
		if c.Kind == "manual" {
			var n int
			s.DB.QueryRow(`SELECT COUNT(*) FROM collection_items WHERE collection_id=? AND item_id=?`, c.ID, itemID).Scan(&n)
			if n > 0 {
				out = append(out, CollectionRef{c.ID, c.Name, c.Kind})
			}
			continue
		}
		if c.Rules == nil {
			continue
		}
		where, args, _, err := c.Rules.Compile(p.ID)
		if err != nil {
			continue
		}
		rf, rargs := RatingSQL(p, "i")
		var n int
		q := `SELECT COUNT(*) FROM items i LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.profile_id=? WHERE i.id=?` + where + rf
		s.DB.QueryRow(q, append(append([]any{p.ID, itemID}, args...), rargs...)...).Scan(&n)
		if n > 0 {
			out = append(out, CollectionRef{c.ID, c.Name, c.Kind})
		}
	}
	return out
}
