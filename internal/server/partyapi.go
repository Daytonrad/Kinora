package server

import (
	"net/http"

	"github.com/ScotyLucas/kinora/internal/party"
)

func (s *Server) handlePartyCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		ItemID   int64 `json:"item_id"`
		Everyone bool  `json:"everyone"`
	}
	if !decode(w, r, &in) {
		return
	}
	it, err := s.st.Item(in.ItemID, c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	if it.Kind != "movie" && it.Kind != "episode" {
		writeErr(w, http.StatusBadRequest, "watch partyt csak filmhez vagy epizódhoz lehet indítani")
		return
	}
	room := s.Party.Create(it.ID, in.Everyone, c.User.ID)
	writeJSON(w, http.StatusCreated, room.Info())
}

func (s *Server) partyRoom(w http.ResponseWriter, r *http.Request, c *reqCtx) *party.Room {
	room := s.Party.Get(r.PathValue("code"))
	if room == nil {
		writeErr(w, http.StatusNotFound, "nincs ilyen watch party (lejárt vagy elírtad a kódot)")
		return nil
	}
	if !s.st.ItemVisible(room.Info().ItemID, c.Profile) {
		writeErr(w, http.StatusForbidden, "ez a watch party a profilod korhatár-beállítása miatt nem érhető el")
		return nil
	}
	return room
}

func (s *Server) handlePartyInfo(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if room := s.partyRoom(w, r, c); room != nil {
		writeJSON(w, http.StatusOK, room.Info())
	}
}

func (s *Server) handlePartyWS(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	room := s.partyRoom(w, r, c)
	if room == nil {
		return
	}
	s.Party.Serve(w, r, room, party.Who{UserID: c.User.ID, ProfileID: c.Profile.ID, Name: c.Profile.Name, Color: c.Profile.Color})
}
