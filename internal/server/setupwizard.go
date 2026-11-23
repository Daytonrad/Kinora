package server

import (
	"net"
	"net/http"
)

// The first-run wizard (web client, #/setup/…) walks a new admin through media folders, metadata, the encoder
// test, downloads and remote access. Every step is optional and uses the regular admin endpoints; these two only
// summarise what is set up and remember that the wizard was closed.

// settingWizard is "0" from the first setup until the wizard is finished or closed ("1"). Installs from before
// the wizard have no value, so their dashboard does not suddenly ask for it.
const settingWizard = "setup_wizard_done"

type setupStatus struct {
	Pending    bool   `json:"pending"` // a new install that has not closed the wizard yet
	ServerName string `json:"server_name"`
	Libraries  int    `json:"libraries"`
	Movies     int    `json:"movies"`
	Series     int    `json:"series"`
	Episodes   int    `json:"episodes"`
	Metadata   struct {
		TMDB bool   `json:"tmdb"` // a TMDB key is set (in the settings or the environment)
		Lang string `json:"lang"`
	} `json:"metadata"`
	Encoder struct {
		Tested   bool   `json:"tested"`
		Name     string `json:"name"`
		Label    string `json:"label"`
		Hardware bool   `json:"hardware"`
	} `json:"encoder"`
	Downloads struct {
		Enabled  bool `json:"enabled"`
		Clients  int  `json:"clients"`
		Indexers int  `json:"indexers"`
	} `json:"downloads"`
	Remote struct {
		PublicURL string `json:"public_url"`
		Tailscale bool   `json:"tailscale"` // the server has a Tailscale (100.64.0.0/10) address
	} `json:"remote"`
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var st setupStatus
	st.Pending = s.st.Setting(settingWizard) == "0"
	st.ServerName = s.st.Setting("server_name")
	libs, err := s.st.Libraries()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	st.Libraries = len(libs)
	ls := s.st.Stats()
	st.Movies, st.Series, st.Episodes = ls.Movies, ls.Series, ls.Episodes
	st.Metadata.TMDB = s.st.Setting("tmdb_key") != ""
	st.Metadata.Lang = s.st.Setting("metadata_lang")
	_, at := s.Encoders.Results()
	enc := s.Encoders.Pick(s.encoderSetting())
	st.Encoder.Tested = !at.IsZero()
	st.Encoder.Name, st.Encoder.Label, st.Encoder.Hardware = enc.Name, enc.Label, enc.Hardware
	st.Downloads.Enabled = s.st.Setting("acq_enabled") == "1"
	if ixs, err := s.st.AcqIndexers(); err == nil {
		st.Downloads.Indexers = len(ixs)
	}
	if cls, err := s.st.AcqClients(); err == nil {
		st.Downloads.Clients = len(cls)
	}
	st.Remote.PublicURL = s.st.Setting("public_url")
	st.Remote.Tailscale = hasTailscaleAddr()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleSetupFinish(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if err := s.st.SetSetting(settingWizard, "1"); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}

// hasTailscaleAddr reports whether an interface of this machine is on Tailscale's 100.64.0.0/10 range.
func hasTailscaleAddr() bool {
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		as, _ := ifc.Addrs()
		for _, a := range as {
			if ipn, ok := a.(*net.IPNet); ok && isTailscaleIP(ipn.IP) {
				return true
			}
		}
	}
	return false
}

func isTailscaleIP(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xC0 == 64
}
