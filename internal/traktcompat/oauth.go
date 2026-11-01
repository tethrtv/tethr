package traktcompat

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"github.com/tethrtv/tethr/internal/core"
)

type oauthRequest struct {
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	CodeVerifier string `json:"code_verifier"`
	Token        string `json:"token"`
}

func readOAuth(w http.ResponseWriter, r *http.Request) (oauthRequest, bool) {
	var in oauthRequest
	if media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); media == "application/x-www-form-urlencoded" {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "That form couldn't be read.")
			return in, false
		}
		f := r.PostForm
		in = oauthRequest{
			Code:         f.Get("code"),
			ClientID:     f.Get("client_id"),
			ClientSecret: f.Get("client_secret"),
			RedirectURI:  f.Get("redirect_uri"),
			GrantType:    f.Get("grant_type"),
			RefreshToken: f.Get("refresh_token"),
			CodeVerifier: f.Get("code_verifier"),
			Token:        f.Get("token"),
		}
		return in, true
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "That request body couldn't be read.")
		return in, false
	}
	return in, true
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"`
}

func writeTokens(w http.ResponseWriter, t core.Tokens) {
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  t.Access,
		TokenType:    "bearer",
		ExpiresIn:    t.ExpiresIn,
		RefreshToken: t.Refresh,
		Scope:        t.Scope,
		CreatedAt:    t.CreatedAt.Unix(),
	})
}

func (s *Server) deviceCode(w http.ResponseWriter, r *http.Request) {
	in, ok := readOAuth(w, r)
	if !ok {
		return
	}
	d, err := core.StartDeviceCode(r.Context(), s.DB, in.ClientID)
	if errors.Is(err, core.ErrUnknownApp) {
		writeError(w, http.StatusForbidden, "invalid_client", "Tethr doesn't know that client ID.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":      d.Code,
		"user_code":        d.UserCode,
		"verification_url": s.AppURL + "/activate",
		"expires_in":       d.ExpiresIn,
		"interval":         d.Interval,
	})
}

func (s *Server) deviceToken(w http.ResponseWriter, r *http.Request) {
	in, ok := readOAuth(w, r)
	if !ok {
		return
	}
	tokens, err := core.PollDeviceCode(r.Context(), s.DB, in.ClientID, in.ClientSecret, in.Code, r.UserAgent())
	switch {
	case errors.Is(err, core.ErrDevicePending):
		writeError(w, http.StatusBadRequest, "authorization_pending", "Waiting for someone to enter the code.")
	case errors.Is(err, core.ErrDeviceSlowDown):
		writeError(w, http.StatusTooManyRequests, "slow_down", "Polling too quickly. Wait for the interval between requests.")
	case errors.Is(err, core.ErrUnknownDevice):
		writeError(w, http.StatusNotFound, "invalid_device_code", "That device code isn't valid.")
	case errors.Is(err, core.ErrDeviceUsed):
		writeError(w, http.StatusConflict, "already_used", "That device code has already been used.")
	case errors.Is(err, core.ErrDeviceExpired):
		writeError(w, http.StatusGone, "expired_token", "That device code has expired. Ask for a new one.")
	case errors.Is(err, core.ErrDeviceDenied):
		writeError(w, http.StatusTeapot, "access_denied", "The code was denied.")
	case errors.Is(err, core.ErrUnknownApp), errors.Is(err, core.ErrBadSecret):
		writeError(w, http.StatusUnauthorized, "invalid_client", "Tethr doesn't know that client.")
	case err != nil:
		s.fail(w, r, err)
	default:
		writeTokens(w, tokens)
	}
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	in, ok := readOAuth(w, r)
	if !ok {
		return
	}
	var tokens core.Tokens
	var err error
	switch in.GrantType {
	case "authorization_code":
		tokens, err = core.ExchangeCode(r.Context(), s.DB, in.ClientID, in.ClientSecret, in.Code, in.RedirectURI, in.CodeVerifier, r.UserAgent())
	case "refresh_token":
		tokens, err = core.RefreshTokens(r.Context(), s.DB, in.ClientID, in.ClientSecret, in.RefreshToken)
	default:
		writeError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token.")
		return
	}
	switch {
	case errors.Is(err, core.ErrUnknownApp), errors.Is(err, core.ErrBadSecret):
		writeError(w, http.StatusUnauthorized, "invalid_client", "Tethr doesn't know that client, or the secret is wrong.")
	case errors.Is(err, core.ErrInvalidGrant):
		writeError(w, http.StatusUnauthorized, "invalid_grant", "That code or refresh token is invalid, expired or already used.")
	case err != nil:
		s.fail(w, r, err)
	default:
		writeTokens(w, tokens)
	}
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	in, ok := readOAuth(w, r)
	if !ok {
		return
	}
	err := core.RevokeToken(r.Context(), s.DB, in.ClientID, in.Token)
	if errors.Is(err, core.ErrUnknownApp) {
		writeError(w, http.StatusUnauthorized, "invalid_client", "Tethr doesn't know that client ID.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.AppURL+"/authorize?"+r.URL.RawQuery, http.StatusFound)
}
