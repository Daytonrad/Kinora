package server

import "net/http"

func (s *Server) routes() {
	m := s.mux
	// public
	m.HandleFunc("GET /api/status", s.public(s.handleStatus))
	m.HandleFunc("POST /api/setup", s.public(s.handleSetup))
	m.HandleFunc("POST /api/login", s.public(s.handleLogin))
	m.HandleFunc("POST /api/logout", s.public(s.handleLogout))
	m.HandleFunc("GET /api/invites/{code}", s.public(s.handleInviteInfo))
	m.HandleFunc("POST /api/join", s.public(s.handleJoin))

	// authed
	m.HandleFunc("GET /api/me", s.authed(s.handleMe))
	m.HandleFunc("POST /api/me/password", s.authed(s.handlePassword))
	m.HandleFunc("POST /api/profiles", s.authed(s.handleProfileCreate))
	m.HandleFunc("PATCH /api/profiles/{id}", s.authed(s.handleProfileUpdate))
	m.HandleFunc("DELETE /api/profiles/{id}", s.authed(s.handleProfileDelete))
	m.HandleFunc("POST /api/profiles/{id}/select", s.authed(s.handleProfileSelect))
	m.HandleFunc("POST /api/profiles/leave", s.authed(s.handleProfileLeave))

	// with profile
	m.HandleFunc("GET /api/libraries", s.withProfile(s.handleLibraries))
	m.HandleFunc("GET /api/images/{id}/{kind}", s.authed(s.handleImage))

	// admin
	m.HandleFunc("GET /api/admin/users", s.admin(s.handleUsers))
	m.HandleFunc("POST /api/admin/users", s.admin(s.handleUserCreate))
	m.HandleFunc("PATCH /api/admin/users/{id}", s.admin(s.handleUserUpdate))
	m.HandleFunc("DELETE /api/admin/users/{id}", s.admin(s.handleUserDelete))
	m.HandleFunc("GET /api/admin/invites", s.admin(s.handleInvites))
	m.HandleFunc("POST /api/admin/invites", s.admin(s.handleInviteCreate))
	m.HandleFunc("DELETE /api/admin/invites/{code}", s.admin(s.handleInviteDelete))
	m.HandleFunc("GET /api/admin/settings", s.admin(s.handleSettings))
	m.HandleFunc("PATCH /api/admin/settings", s.admin(s.handleSettingsPatch))
	m.HandleFunc("GET /api/admin/backup", s.admin(s.handleBackup))
	m.HandleFunc("GET /api/admin/connect", s.admin(s.handleConnect))
	m.HandleFunc("GET /api/admin/browse", s.admin(s.handleBrowse))
	m.HandleFunc("GET /api/admin/dashboard", s.admin(s.handleDashboard))
	m.HandleFunc("POST /api/libraries", s.admin(s.handleLibraryCreate))
	m.HandleFunc("DELETE /api/libraries/{id}", s.admin(s.handleLibraryDelete))
	m.HandleFunc("POST /api/libraries/{id}/scan", s.admin(s.handleLibraryScan))
	m.HandleFunc("GET /api/admin/scan", s.admin(s.handleScanStatus))

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "ismeretlen API végpont")
	})
	m.Handle("/", s.webHandler())
}

