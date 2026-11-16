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
	m.HandleFunc("GET /api/home", s.withProfile(s.handleHome))
	m.HandleFunc("GET /api/items", s.withProfile(s.handleItems))
	m.HandleFunc("GET /api/items/{id}", s.withProfile(s.handleItem))
	m.HandleFunc("GET /api/items/{id}/children", s.withProfile(s.handleChildren))
	m.HandleFunc("GET /api/items/{id}/next", s.withProfile(s.handleNext))
	m.HandleFunc("POST /api/items/{id}/{action}", s.withProfile(s.handleItemAction))
	m.HandleFunc("PUT /api/items/{id}/tags", s.withProfile(notKids(s.handleTags)))
	m.HandleFunc("GET /api/search", s.withProfile(s.handleSearch))
	m.HandleFunc("GET /api/facets", s.withProfile(s.handleFacets))
	m.HandleFunc("GET /api/collections", s.withProfile(s.handleCollections))
	m.HandleFunc("POST /api/collections", s.withProfile(notKids(s.handleCollectionCreate)))
	m.HandleFunc("POST /api/collections/preview", s.withProfile(s.handleCollectionPreview))
	m.HandleFunc("GET /api/collections/{id}", s.withProfile(s.handleCollection))
	m.HandleFunc("PATCH /api/collections/{id}", s.withProfile(notKids(s.handleCollectionUpdate)))
	m.HandleFunc("DELETE /api/collections/{id}", s.withProfile(notKids(s.handleCollectionDelete)))
	m.HandleFunc("PUT /api/collections/{id}/order", s.withProfile(notKids(s.handleCollectionOrder)))
	m.HandleFunc("POST /api/collections/{id}/items/{item}", s.withProfile(notKids(s.handleCollectionItem)))
	m.HandleFunc("DELETE /api/collections/{id}/items/{item}", s.withProfile(notKids(s.handleCollectionItem)))
	m.HandleFunc("GET /api/images/{id}/{kind}", s.authed(s.handleImage))
	m.HandleFunc("GET /api/hls/{session}/{file}", s.authed(s.handleHLS))
	m.HandleFunc("DELETE /api/hls/{session}", s.authed(s.handleHLSStop))
	m.HandleFunc("POST /api/items/{id}/playback", s.withProfile(s.handlePlayback))
	m.HandleFunc("GET /api/stream/{id}", s.withProfile(s.handleStream))
	m.HandleFunc("GET /api/items/{id}/subtitles/{idx}", s.withProfile(s.handleSubtitle))
	m.HandleFunc("POST /api/items/{id}/download", s.withProfile(s.handleDownloadCreate))
	m.HandleFunc("GET /api/downloads", s.withProfile(s.handleDownloads))
	m.HandleFunc("GET /api/downloads/{key}/file", s.withProfile(s.handleDownloadFile))

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
	m.HandleFunc("POST /api/admin/hwtest", s.admin(s.handleHWTest))

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "ismeretlen API végpont")
	})
	m.Handle("/", s.webHandler())
}

