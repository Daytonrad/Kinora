package server

import (
	"fmt"
	"testing"
)

type accountsResp struct {
	Accounts []struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
		Current  bool   `json:"current"`
		Profiles []struct {
			ID int64 `json:"id"`
		} `json:"profiles"`
	} `json:"accounts"`
}

type meResp struct {
	User    struct{ Username string } `json:"user"`
	Profile *struct{ ID int64 }       `json:"profile"`
}

// Two people sign in on the same browser: both stay signed in, and switching is one call.
func TestRememberedAccountsSwitch(t *testing.T) {
	c, _ := newTestServer(t) // signed in as admin
	if code := c.do("POST", "/api/admin/users", map[string]any{"username": "bela", "password": "secret123"}, nil); code != 201 && code != 200 {
		t.Fatalf("create user %d", code)
	}
	// "add user": bela signs in while admin is still signed in
	if code := c.do("POST", "/api/login", map[string]string{"username": "bela", "password": "secret123"}, nil); code != 200 {
		t.Fatalf("login bela %d", code)
	}
	var acc accountsResp
	c.do("GET", "/api/accounts", nil, &acc)
	if len(acc.Accounts) != 2 || acc.Accounts[0].Username != "bela" || !acc.Accounts[0].Current || acc.Accounts[1].Username != "admin" {
		t.Fatalf("accounts after second login: %+v", acc)
	}
	admin := acc.Accounts[1]

	// switch to admin straight into a profile, no password
	if code := c.do("POST", fmt.Sprintf("/api/accounts/%d/switch", admin.UserID), map[string]any{"profile_id": admin.Profiles[0].ID}, nil); code != 200 {
		t.Fatalf("switch %d", code)
	}
	var me meResp
	c.do("GET", "/api/me", nil, &me)
	if me.User.Username != "admin" || me.Profile == nil || me.Profile.ID != admin.Profiles[0].ID {
		t.Fatalf("after switch: %+v", me)
	}
	c.do("GET", "/api/accounts", nil, &acc)
	if len(acc.Accounts) != 2 || acc.Accounts[0].Username != "admin" || acc.Accounts[1].Username != "bela" {
		t.Fatalf("accounts after switch: %+v", acc)
	}

	// signing in again as the same person does not add a duplicate
	c.do("POST", "/api/login", map[string]string{"username": "admin", "password": "secret123"}, nil)
	c.do("GET", "/api/accounts", nil, &acc)
	if len(acc.Accounts) != 2 {
		t.Fatalf("duplicate account: %+v", acc)
	}

	// a profile of someone else that is not remembered here cannot be switched to
	if code := c.do("POST", "/api/accounts/999/switch", map[string]any{}, nil); code != 404 {
		t.Errorf("switch to unknown account: %d", code)
	}

	// signing out hands over to the other remembered account, on the profile picker
	var out struct{ Switched bool }
	c.do("POST", "/api/logout", nil, &out)
	if !out.Switched {
		t.Fatal("logout did not switch to the remembered account")
	}
	me = meResp{}
	c.do("GET", "/api/me", nil, &me)
	if me.User.Username != "bela" || me.Profile != nil {
		t.Fatalf("after logout: %+v", me)
	}
	// forgetting the last one is a plain sign-out
	c.do("POST", "/api/logout", nil, &out)
	if out.Switched {
		t.Error("switched with nobody left")
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Errorf("still signed in: %d", code)
	}
}

func TestForgetRememberedAccount(t *testing.T) {
	c, _ := newTestServer(t)
	c.do("POST", "/api/admin/users", map[string]any{"username": "bela", "password": "secret123"}, nil)
	c.do("POST", "/api/login", map[string]string{"username": "bela", "password": "secret123"}, nil)
	var acc accountsResp
	c.do("GET", "/api/accounts", nil, &acc)
	if len(acc.Accounts) != 2 {
		t.Fatalf("accounts: %+v", acc)
	}
	if code := c.do("DELETE", fmt.Sprintf("/api/accounts/%d", acc.Accounts[1].UserID), nil, nil); code != 200 {
		t.Fatalf("forget %d", code)
	}
	c.do("GET", "/api/accounts", nil, &acc)
	if len(acc.Accounts) != 1 || acc.Accounts[0].Username != "bela" {
		t.Fatalf("after forget: %+v", acc)
	}
}
