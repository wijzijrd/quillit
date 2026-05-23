package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type CampaignsHandler struct{ db *sql.DB }

func NewCampaigns(db *sql.DB) *CampaignsHandler { return &CampaignsHandler{db: db} }

type Campaign struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	CreatedAt int64   `json:"createdAt"`
	Players   []Player `json:"players"`
}

type Player struct {
	ID         string `json:"id"`
	CampaignID string `json:"campaignId"`
	Name       string `json:"name"`
	Token      string `json:"token"`
	CreatedAt  int64  `json:"createdAt"`
}

// List godoc
// @Summary      List campaigns
// @Tags         campaigns
// @Produce      json
// @Success      200  {array}   Campaign
// @Failure      500  {object}  ErrorResponse
// @Router       /api/campaigns [get]
func (h *CampaignsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), "SELECT id, name, created_at FROM campaigns ORDER BY created_at ASC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	campaigns := []Campaign{}
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		c.Players = h.playersFor(r, c.ID)
		campaigns = append(campaigns, c)
	}
	writeJSON(w, http.StatusOK, campaigns)
}

// CreateCampaignRequest is the body for creating a campaign.
type CreateCampaignRequest struct {
	Name string `json:"name"`
}

// Create godoc
// @Summary      Create campaign
// @Tags         campaigns
// @Accept       json
// @Produce      json
// @Param        body  body      CreateCampaignRequest  true  "Campaign name"
// @Success      201   {object}  Campaign
// @Failure      400   {object}  ErrorResponse
// @Router       /api/campaigns [post]
func (h *CampaignsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	c := Campaign{ID: newID(), Name: body.Name, CreatedAt: nowUnix()}
	if _, err := h.db.ExecContext(r.Context(), "INSERT INTO campaigns (id, name, created_at) VALUES (?, ?, ?)", c.ID, c.Name, c.CreatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	c.Players = []Player{}
	writeJSON(w, http.StatusCreated, c)
}

// Update godoc
// @Summary      Update campaign
// @Tags         campaigns
// @Accept       json
// @Produce      json
// @Param        id    path      string                 true  "Campaign ID"
// @Param        body  body      CreateCampaignRequest  true  "Campaign name"
// @Success      200   {object}  Campaign
// @Failure      404   {object}  ErrorResponse
// @Router       /api/campaigns/{id} [patch]
func (h *CampaignsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	res, err := h.db.ExecContext(r.Context(), "UPDATE campaigns SET name = ? WHERE id = ?", body.Name, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var c Campaign
	_ = h.db.QueryRowContext(r.Context(), "SELECT id, name, created_at FROM campaigns WHERE id = ?", id).Scan(&c.ID, &c.Name, &c.CreatedAt)
	c.Players = h.playersFor(r, id)
	writeJSON(w, http.StatusOK, c)
}

// Delete godoc
// @Summary      Delete campaign
// @Tags         campaigns
// @Produce      json
// @Param        id   path      string      true  "Campaign ID"
// @Success      200  {object}  OkResponse
// @Router       /api/campaigns/{id} [delete]
func (h *CampaignsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM campaigns WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AddPlayerRequest is the body for adding a player to a campaign.
type AddPlayerRequest struct {
	Name string `json:"name"`
}

// AddPlayer godoc
// @Summary      Add player to campaign
// @Tags         campaigns
// @Accept       json
// @Produce      json
// @Param        campaignId  path      string            true  "Campaign ID"
// @Param        body        body      AddPlayerRequest  true  "Player name"
// @Success      201         {object}  Player
// @Failure      404         {object}  ErrorResponse
// @Router       /api/campaigns/{campaignId}/players [post]
func (h *CampaignsHandler) AddPlayer(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	var body struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	var exists int
	_ = h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM campaigns WHERE id = ?", campaignID).Scan(&exists)
	if exists == 0 {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	token := newID()
	p := Player{ID: newID(), CampaignID: campaignID, Name: body.Name, Token: token, CreatedAt: nowUnix()}
	if _, err := h.db.ExecContext(r.Context(), "INSERT INTO players (id, campaign_id, name, token, created_at) VALUES (?, ?, ?, ?, ?)", p.ID, p.CampaignID, p.Name, p.Token, p.CreatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// RemovePlayer godoc
// @Summary      Remove player from campaign
// @Tags         campaigns
// @Produce      json
// @Param        campaignId  path      string      true  "Campaign ID"
// @Param        playerId    path      string      true  "Player ID"
// @Success      200         {object}  OkResponse
// @Router       /api/campaigns/{campaignId}/players/{playerId} [delete]
func (h *CampaignsHandler) RemovePlayer(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM players WHERE id = ?", playerID); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *CampaignsHandler) playersFor(r *http.Request, campaignID string) []Player {
	rows, err := h.db.QueryContext(r.Context(), "SELECT id, campaign_id, name, token, created_at FROM players WHERE campaign_id = ? ORDER BY created_at ASC", campaignID)
	if err != nil {
		return []Player{}
	}
	defer rows.Close()
	players := []Player{}
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.CampaignID, &p.Name, &p.Token, &p.CreatedAt); err == nil {
			players = append(players, p)
		}
	}
	return players
}

var _ = errors.New // keep import
