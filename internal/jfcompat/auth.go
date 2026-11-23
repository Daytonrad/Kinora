package jfcompat

import (
	"net/http"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

func (s *Server) serverID() string { return s.ids.encode(nsServer, 1) }

// ServerID is the id this API reports (also used in the LAN discovery answer).
func (s *Server) ServerID() string { return s.serverID() }

func (s *Server) serverName() string {
	if n := strings.TrimSpace(s.Store.Setting("server_name")); n != "" {
		return n
	}
	return "Kinora"
}

func localAddress(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// ------------------------------------------------------------------ system

func (s *Server) publicInfo(r *http.Request) map[string]any {
	return map[string]any{
		"LocalAddress": localAddress(r), "ServerName": s.serverName(), "Version": APIVersion, "ProductName": "Kinora",
		"OperatingSystem": "", "Id": s.serverID(), "StartupWizardCompleted": true,
	}
}

func (s *Server) handleInfoPublic(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, s.publicInfo(r))
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	info := s.publicInfo(r)
	for k, v := range map[string]any{
		"OperatingSystemDisplayName": "", "HasPendingRestart": false, "IsShuttingDown": false, "SupportsLibraryMonitor": true,
		"WebSocketPortNumber": 0, "CompletedInstallations": []any{}, "CanSelfRestart": false, "CanLaunchWebBrowser": false,
		"HasUpdateAvailable": false, "TranscodingTempPath": "", "LogPath": "", "InternalMetadataPath": "", "CachePath": "",
		"ProgramDataPath": "", "ItemsByNamePath": "", "EncoderLocation": "System", "SystemArchitecture": "",
	} {
		info[k] = v
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, "Kinora")
}

func (s *Server) handleBranding(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, map[string]any{"LoginDisclaimer": "", "CustomCss": "", "SplashscreenEnabled": false})
}

// ------------------------------------------------------------------ users

// userName is how a profile signs in: the account name for its default profile, "account/profile" otherwise.
func (s *Server) userName(u *store.User, p *store.Profile) string {
	if def := s.defaultProfile(u.ID); def != nil && def.ID == p.ID {
		return u.Username
	}
	return u.Username + "/" + p.Name
}

// defaultProfile is the profile a plain account name signs in to: the oldest adult profile (or the oldest one).
func (s *Server) defaultProfile(userID int64) *store.Profile {
	ps, err := s.Store.Profiles(userID)
	if err != nil || len(ps) == 0 {
		return nil
	}
	var best *store.Profile
	for _, p := range ps {
		if !p.IsKids && (best == nil || p.ID < best.ID) {
			best = p
		}
	}
	if best == nil {
		for _, p := range ps {
			if best == nil || p.ID < best.ID {
				best = p
			}
		}
	}
	return best
}

func (s *Server) userDTO(u *store.User, p *store.Profile) map[string]any {
	firstLang := func(list string) string {
		for _, l := range strings.Split(list, ",") {
			if l = strings.TrimSpace(l); l != "" {
				return l
			}
		}
		return ""
	}
	subMode := map[string]string{"off": "None", "always": "Always", "auto": "Smart"}[p.SubMode]
	if subMode == "" {
		subMode = "Default"
	}
	return map[string]any{
		"Name": s.userName(u, p), "ServerId": s.serverID(), "Id": s.ids.encode(nsUser, p.ID),
		"HasPassword": true, "HasConfiguredPassword": true, "HasConfiguredEasyPassword": false, "EnableAutoLogin": false,
		"LastLoginDate": isoTime(time.Now().Unix()), "LastActivityDate": isoTime(time.Now().Unix()),
		"Configuration": map[string]any{
			"AudioLanguagePreference": firstLang(p.AudioLangs), "PlayDefaultAudioTrack": true,
			"SubtitleLanguagePreference": firstLang(p.SubLangs), "SubtitleMode": subMode,
			"DisplayMissingEpisodes": false, "GroupedFolders": []string{}, "OrderedViews": []string{}, "LatestItemsExcludes": []string{},
			"MyMediaExcludes": []string{}, "HidePlayedInLatest": false, "RememberAudioSelections": true, "RememberSubtitleSelections": true,
			"EnableNextEpisodeAutoPlay": true, "DisplayCollectionsView": false, "EnableLocalPassword": false,
		},
		"Policy": map[string]any{
			"IsAdministrator": false, "IsHidden": true, "IsDisabled": false, "EnableUserPreferenceAccess": false,
			"EnableRemoteControlOfOtherUsers": false, "EnableSharedDeviceControl": false, "EnableRemoteAccess": true,
			"EnableLiveTvManagement": false, "EnableLiveTvAccess": false, "EnableMediaPlayback": true,
			"EnableAudioPlaybackTranscoding": true, "EnableVideoPlaybackTranscoding": true, "EnablePlaybackRemuxing": true,
			"ForceRemoteSourceTranscoding": false, "EnableContentDeletion": false, "EnableContentDownloading": true,
			"EnableSyncTranscoding": false, "EnableMediaConversion": false, "EnableAllDevices": true, "EnableAllChannels": false,
			"EnableAllFolders": true, "EnabledFolders": []string{}, "EnablePublicSharing": false, "BlockedTags": []string{},
			"AllowedTags": []string{}, "BlockUnratedItems": []string{}, "EnabledDevices": []string{}, "EnabledChannels": []string{},
			"InvalidLoginAttemptCount": 0, "LoginAttemptsBeforeLockout": maxFails, "MaxActiveSessions": 0,
			"RemoteClientBitrateLimit": p.MaxBitrate * 1000, "AuthenticationProviderId": "Kinora", "PasswordResetProviderId": "Kinora",
			"SyncPlayAccess": "None",
		},
	}
}

func (s *Server) handleUsersPublic(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, []any{}) // nobody is listed on a client's login screen
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, s.userDTO(c.User, c.Profile))
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if id, ok := s.ids.decode(nsUser, r.PathValue("user")); !ok || id != c.Profile.ID {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, s.userDTO(c.User, c.Profile))
}

// ------------------------------------------------------------------ sign in

// handleAuthByName: Username "account" or "account/profile"; Pw is the account password, with "#PIN" at the
// end for a profile that has a PIN. Five wrong tries per account and IP lock the pair out for 15 minutes.
func (s *Server) handleAuthByName(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Username string `json:"Username"`
		Pw       string `json:"Pw"`
		Password string `json:"Password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	pw := in.Pw
	if pw == "" {
		pw = in.Password
	}
	acct, profName, _ := strings.Cut(strings.TrimSpace(in.Username), "/")
	acct = strings.TrimSpace(acct)
	ip := clientIP(r)
	key, ipKey := "acct:"+strings.ToLower(acct)+"|"+ip, "ip:"+ip
	if s.locks.locked(key, ipKey) {
		store.DummyCheck(pw)
		writeErr(w, http.StatusTooManyRequests, "too many failed sign-ins, try again in 15 minutes")
		return
	}
	fail := func() {
		s.locks.fail(key, maxFails)
		s.locks.fail(ipKey, maxIPFails)
		s.Log.Info("jellyfin-compat: sign-in failed", "account", acct, "ip", ip)
		writeErr(w, http.StatusUnauthorized, "invalid user name or password")
	}
	u, err := s.Store.UserByName(acct)
	if err != nil {
		store.DummyCheck(pw)
		fail()
		return
	}
	var p *store.Profile
	if profName = strings.TrimSpace(profName); profName == "" {
		p = s.defaultProfile(u.ID)
	} else if ps, err := s.Store.Profiles(u.ID); err == nil {
		for _, x := range ps {
			if strings.EqualFold(x.Name, profName) {
				p = x
			}
		}
	}
	if p == nil {
		store.DummyCheck(pw)
		fail()
		return
	}
	accountPw, pin := pw, ""
	if p.HasPIN {
		i := strings.LastIndexByte(pw, '#')
		if i < 0 {
			store.DummyCheck(pw)
			fail()
			return
		}
		accountPw, pin = pw[:i], pw[i+1:]
	}
	if !s.Store.CheckUserPassword(u, accountPw) || (p.HasPIN && !p.CheckPIN(pin)) {
		fail()
		return
	}
	s.locks.reset(key)
	s.signIn(w, r, c, u, p)
}

// signIn creates a device session pinned to the profile and answers with Jellyfin's AuthenticationResult.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request, c *reqCtx, u *store.User, p *store.Profile) {
	name := firstNonEmpty(c.Auth.Device, c.Auth.Client, "Jellyfin client")
	d := store.DeviceInfo{Name: name, Platform: Platform, AppVersion: strings.TrimSpace(c.Auth.Client + " " + c.Auth.Version), IP: clientIP(r)}
	tok, dev, err := s.Store.CreateDeviceSession(u.ID, d, p.ID, p.ID)
	if err != nil {
		s.Log.Error("jellyfin-compat: session", "err", err)
		writeErr(w, http.StatusInternalServerError, "could not create the session")
		return
	}
	s.Log.Info("jellyfin-compat: signed in", "account", u.Username, "profile", p.Name, "client", c.Auth.Client, "device", name)
	user := s.userDTO(u, p)
	writeJSON(w, http.StatusOK, map[string]any{
		"User": user, "AccessToken": tok, "ServerId": s.serverID(),
		"SessionInfo": map[string]any{
			"Id": dev, "UserId": user["Id"], "UserName": user["Name"], "Client": c.Auth.Client, "DeviceName": name,
			"DeviceId": firstNonEmpty(c.Auth.DeviceID, dev), "ApplicationVersion": c.Auth.Version, "IsActive": true,
			"SupportsMediaControl": false, "SupportsRemoteControl": false, "PlayableMediaTypes": []string{"Video"},
			"ServerId": s.serverID(), "LastActivityDate": isoTime(time.Now().Unix()), "AdditionalUsers": []any{},
		},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.Store.DeleteSession(c.Token)
	w.WriteHeader(http.StatusNoContent)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// ------------------------------------------------------------------ Quick Connect

func (s *Server) handleQCEnabled(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, s.QC != nil && s.QC.Enabled())
}

func (s *Server) qcResult(secret, code string, c *reqCtx, done bool) map[string]any {
	return map[string]any{
		"Authenticated": done, "Secret": secret, "Code": code, "DeviceId": c.Auth.DeviceID, "DeviceName": c.Auth.Device,
		"AppName": c.Auth.Client, "AppVersion": c.Auth.Version, "DateAdded": isoTime(time.Now().Unix()),
	}
}

func (s *Server) handleQCInitiate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if s.QC == nil || !s.QC.Enabled() {
		writeErr(w, http.StatusUnauthorized, "Quick Connect is switched off")
		return
	}
	d := store.DeviceInfo{Name: firstNonEmpty(c.Auth.Device, c.Auth.Client, "Jellyfin client"), Platform: Platform,
		AppVersion: strings.TrimSpace(c.Auth.Client + " " + c.Auth.Version), IP: clientIP(r)}
	code, secret, err := s.QC.Initiate(d)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.qcResult(secret, code, c, false))
}

func (s *Server) handleQCConnect(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if s.QC == nil {
		writeErr(w, http.StatusNotFound, "unknown secret")
		return
	}
	secret := qv(r, "Secret")
	switch s.QC.State(secret) {
	case "pending":
		writeJSON(w, http.StatusOK, s.qcResult(secret, "", c, false))
	case "approved":
		writeJSON(w, http.StatusOK, s.qcResult(secret, "", c, true))
	default:
		writeErr(w, http.StatusNotFound, "unknown or expired secret")
	}
}

func (s *Server) handleAuthQC(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Secret string `json:"Secret"`
	}
	if err := decode(r, &in); err != nil || s.QC == nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	uid, pid, ok := s.QC.Claim(in.Secret)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "the code was not approved")
		return
	}
	u, err := s.Store.UserByID(uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "the account no longer exists")
		return
	}
	p := s.defaultProfile(u.ID)
	if pid > 0 {
		if x, err := s.Store.Profile(pid); err == nil && x.UserID == u.ID {
			p = x
		}
	}
	if p == nil {
		writeErr(w, http.StatusUnauthorized, "the account has no profile")
		return
	}
	s.signIn(w, r, c, u, p)
}
