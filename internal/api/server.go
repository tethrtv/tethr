package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/mail"
	"github.com/tethrtv/tethr/internal/store"
)

type Mailer interface {
	Send(ctx context.Context, m mail.Message) error
}

type CookieConfig struct {
	Name   string
	Domain string
	Secure bool
}

type Server struct {
	DB             *pgxpool.Pool
	WebAuthn       *webauthn.WebAuthn
	RequireInvite  bool
	Mail           Mailer
	Log            *slog.Logger
	AppURL         string
	AllowedOrigins []string
	Cookie         CookieConfig
}

type contextKey int

const (
	sessionKey contextKey = iota
	userAgentKey
)

func (s *Server) Handler() http.Handler {
	strict := NewStrictHandlerWithOptions(s, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "That request couldn't be read.")
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			s.log().Error("request failed", "path", r.URL.Path, "err", err)
			writeProblem(w, http.StatusInternalServerError, "server_error", "Something went wrong on our side.")
		},
	})
	return s.origins(s.withRequest(HandlerFromMux(strict, http.NewServeMux())))
}

func (s *Server) withRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userAgentKey, r.UserAgent())
		if c, err := r.Cookie(s.Cookie.Name); err == nil {
			ctx = context.WithValue(ctx, sessionKey, c.Value)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) origins(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := slices.Contains(s.AllowedOrigins, origin)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			if allowed {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && origin != "" && !allowed {
			writeProblem(w, http.StatusForbidden, "forbidden_origin", "Requests from that site aren't allowed.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) CreateAccount(ctx context.Context, req CreateAccountRequestObject) (CreateAccountResponseObject, error) {
	if req.Body == nil {
		return CreateAccount400JSONResponse{problem("invalid_request", "Missing details.")}, nil
	}
	email, err := core.StartSignUp(ctx, s.DB, core.SignUp{Email: req.Body.Email, Name: req.Body.Name, Username: req.Body.Username, Invite: value(req.Body.Invite)}, s.RequireInvite)
	switch {
	case errors.Is(err, core.ErrUsernameTaken):
		return CreateAccount409JSONResponse(usernameTaken), nil
	case errors.Is(err, core.ErrInviteRequired):
		return CreateAccount403JSONResponse(problem("invite_required", "Tethr is invite-only for now. Ask the person who told you about it for a code.")), nil
	case errors.Is(err, core.ErrInvalidInvite):
		return CreateAccount403JSONResponse(problem("invalid_invite", "That invite code isn’t valid or has already been used.")), nil
	case errors.Is(err, core.ErrTooManyEmails):
		return CreateAccount429JSONResponse(problem("too_many_emails", "That’s a lot of emails in a short time. Try again in a few minutes.")), nil
	case errors.Is(err, core.ErrEmailTaken):
		return CreateAccount202Response{}, nil
	case err != nil:
		if p, ok := inputProblem(err); ok {
			return CreateAccount400JSONResponse{p}, nil
		}
		return nil, err
	}
	return CreateAccount202Response{}, s.send(ctx, email)
}

func (s *Server) RequestEmailLink(ctx context.Context, req RequestEmailLinkRequestObject) (RequestEmailLinkResponseObject, error) {
	if req.Body == nil {
		return RequestEmailLink400JSONResponse{problem("invalid_request", "Missing an email address.")}, nil
	}
	email, err := core.StartSignIn(ctx, s.DB, req.Body.Email)
	switch {
	case errors.Is(err, core.ErrTooManyEmails):
		return RequestEmailLink429JSONResponse(problem("too_many_emails", "That’s a lot of emails in a short time. Try again in a few minutes.")), nil
	case err != nil:
		if p, ok := inputProblem(err); ok {
			return RequestEmailLink400JSONResponse{p}, nil
		}
		return nil, err
	}
	if email == nil {
		return RequestEmailLink202Response{}, nil
	}
	return RequestEmailLink202Response{}, s.send(ctx, email)
}

func (s *Server) Verify(ctx context.Context, req VerifyRequestObject) (VerifyResponseObject, error) {
	if req.Body == nil {
		return Verify400JSONResponse{problem("invalid_request", "Missing a link or code.")}, nil
	}
	v := core.Verification{Token: value(req.Body.Token), Email: value(req.Body.Email), Code: value(req.Body.Code)}
	if v.Token == "" && (v.Email == "" || v.Code == "") {
		return Verify400JSONResponse{problem("invalid_request", "Send the token from the link, or the email address and code.")}, nil
	}
	userAgent, _ := ctx.Value(userAgentKey).(string)
	session, err := core.Verify(ctx, s.DB, v, userAgent)
	switch {
	case errors.Is(err, core.ErrInvalidLink):
		return Verify400JSONResponse{problem("invalid_link", "That link or code has expired or has already been used.")}, nil
	case errors.Is(err, core.ErrTooManyAttempts):
		return Verify429JSONResponse(problem("too_many_attempts", "Too many wrong codes. Ask for a new email.")), nil
	case err != nil:
		return nil, err
	}
	accountID, err := core.SessionAccount(ctx, s.DB, session.Token)
	if err != nil {
		return nil, err
	}
	me, err := s.me(ctx, accountID)
	if err != nil {
		return nil, err
	}
	cookie := s.cookie(session.Token, session.ExpiresAt)
	return Verify200JSONResponse{Body: me, Headers: Verify200ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *Server) SignOut(ctx context.Context, _ SignOutRequestObject) (SignOutResponseObject, error) {
	if token, ok := ctx.Value(sessionKey).(string); ok {
		if err := core.SignOut(ctx, s.DB, token); err != nil {
			return nil, err
		}
	}
	cookie := s.cookie("", time.Unix(0, 0))
	return SignOut204Response{Headers: SignOut204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *Server) GetMe(ctx context.Context, _ GetMeRequestObject) (GetMeResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return GetMe401JSONResponse{notSignedIn}, err
	}
	me, err := s.me(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return GetMe200JSONResponse(me), nil
}

func (s *Server) me(ctx context.Context, accountID int64) (Me, error) {
	account, profiles, err := core.Me(ctx, s.DB, accountID)
	if err != nil {
		return Me{}, err
	}
	me := Me{
		Account:  Account{Id: account.ID, Email: account.Email, Name: account.Name, CreatedAt: account.CreatedAt},
		Profiles: make([]Profile, 0, len(profiles)),
	}
	for _, p := range profiles {
		me.Profiles = append(me.Profiles, toProfile(p))
	}
	return me, nil
}

func toProfile(p store.Profile) Profile {
	return Profile{Id: p.ID, Username: p.Username, Name: p.Name, Kind: ProfileKind(p.Kind), Private: p.Private}
}

func (s *Server) cookie(value string, expires time.Time) string {
	c := http.Cookie{
		Name:     s.Cookie.Name,
		Value:    value,
		Path:     "/",
		Domain:   s.Cookie.Domain,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	}
	return c.String()
}

func (s *Server) send(ctx context.Context, email *core.SignInEmail) error {
	m, err := signInEmail(s.AppURL, email)
	if err != nil {
		return err
	}
	return s.Mail.Send(ctx, m)
}

func (s *Server) log() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}

func inputProblem(err error) (ProblemJSONResponse, bool) {
	switch {
	case errors.Is(err, core.ErrInvalidEmail):
		return problem("invalid_email", "That doesn’t look like an email address."), true
	case errors.Is(err, core.ErrInvalidUsername):
		return problem("invalid_username", "Usernames are 2 to 24 lowercase letters, numbers and dashes."), true
	case errors.Is(err, core.ErrNameRequired):
		return problem("name_required", "What should we call you?"), true
	case errors.Is(err, core.ErrInvalidKind):
		return problem("invalid_kind", "A profile is a person, kids or a guest."), true
	}
	return ProblemJSONResponse{}, false
}

func problem(code, message string) ProblemJSONResponse {
	return ProblemJSONResponse{Code: code, Message: message}
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"code":%q,"message":%q}`+"\n", code, message)
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
