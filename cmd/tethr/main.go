package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/api"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/hooks"
	"github.com/tethrtv/tethr/internal/jobs"
	"github.com/tethrtv/tethr/internal/limit"
	"github.com/tethrtv/tethr/internal/mail"
	"github.com/tethrtv/tethr/internal/meta"
	"github.com/tethrtv/tethr/internal/tmdb"
	"github.com/tethrtv/tethr/internal/traktcompat"
	"github.com/tethrtv/tethr/internal/wikidata"
	"github.com/tethrtv/tethr/migrations"
)

const localDatabase = "postgres://tethr:tethr@localhost:5432/tethr?sslmode=disable"

var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	run := serve
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			run = migrate
		case "invite":
			run = invite
		case "app":
			run = app
		case "report":
			run = report
		case "official":
			run = official
		}
	}
	if err := run(log); err != nil {
		log.Error("tethr stopped", "err", err)
		os.Exit(1)
	}
}

func migrate(log *slog.Logger) error {
	results, err := migrations.Up(context.Background(), envOr("DATABASE_URL", localDatabase))
	if err != nil {
		return err
	}
	for _, r := range results {
		log.Info("migrated", "version", r.Source.Version, "file", r.Source.Path)
	}
	log.Info("database is up to date")
	return nil
}

func invite(*slog.Logger) error {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, envOr("DATABASE_URL", localDatabase))
	if err != nil {
		return err
	}
	defer db.Close()
	args := os.Args[2:]
	switch {
	case len(args) == 1 && (args[0] == "pause" || args[0] == "resume"):
		if err := core.PauseInvites(ctx, db, args[0] == "pause"); err != nil {
			return err
		}
		fmt.Printf("Invites %sd. Codes already made still work.\n", args[0])
		return nil
	case len(args) > 0 && args[0] == "grant":
		n, err := strconv.Atoi(strings.Join(args[2:], ""))
		if len(args) != 3 || err != nil || n < 1 {
			return errors.New("usage: tethr invite grant <username> <how many>")
		}
		left, err := core.GrantInvites(ctx, db, args[1], n)
		if err != nil {
			return err
		}
		fmt.Printf("%s can make %d more invites.\n", args[1], left)
		return nil
	}
	code, err := core.CreateInvite(ctx, db, strings.Join(args, " "))
	if err != nil {
		return err
	}
	fmt.Printf("%s\n%s/join?invite=%s\n", code, envOr("APP_URL", "http://localhost:5173"), code)
	return nil
}

func app(*slog.Logger) error {
	if len(os.Args) < 3 {
		return errors.New("usage: tethr app <name> [redirect-uri...]")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, envOr("DATABASE_URL", localDatabase))
	if err != nil {
		return err
	}
	defer db.Close()
	a, err := core.CreateApp(ctx, db, os.Args[2], os.Args[3:])
	if err != nil {
		return err
	}
	fmt.Printf("client id:     %s\nclient secret: %s\n", a.ClientID, a.ClientSecret)
	return nil
}

func official(*slog.Logger) error {
	if len(os.Args) != 3 {
		return errors.New("usage: tethr official <email>")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, envOr("DATABASE_URL", localDatabase))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := core.CreateOfficialAccount(ctx, db, os.Args[2]); err != nil {
		return err
	}
	fmt.Printf("Made @%s for %s. Sign in with that email to make Tethr's lists.\n", core.OfficialUsername, os.Args[2])
	return nil
}

func report(*slog.Logger) error {
	usage := errors.New("usage: tethr report [keep <list-id> | remove <list-id> <reason the owner will read>]")
	ctx := context.Background()
	db, err := pgxpool.New(ctx, envOr("DATABASE_URL", localDatabase))
	if err != nil {
		return err
	}
	defer db.Close()
	args := os.Args[2:]
	if len(args) == 0 {
		reports, err := core.OpenReports(ctx, db)
		if err != nil {
			return err
		}
		if len(reports) == 0 {
			fmt.Println("No reports waiting.")
		}
		for _, r := range reports {
			from := "a deleted profile"
			if r.ReporterUsername != nil {
				from = *r.ReporterUsername
			}
			where := ""
			if r.Hidden {
				where = ", out of browsing"
			}
			fmt.Printf("%d  %q by %s%s: %s, from %s on %s\n", r.ListID, r.ListName, r.OwnerUsername, where, r.Reason, from, r.CreatedAt.Format("2 Jan 15:04"))
			if r.Note != nil {
				fmt.Printf("    %s\n", *r.Note)
			}
		}
		return nil
	}
	if len(args) < 2 {
		return usage
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return usage
	}
	switch {
	case args[0] == "keep":
		if err := core.KeepList(ctx, db, id); err != nil {
			return err
		}
		fmt.Printf("Kept %d. It's back in browsing.\n", id)
	case args[0] == "remove" && len(args) > 2:
		removed, err := core.RemoveList(ctx, db, id)
		if err != nil {
			return err
		}
		m, err := api.RemovedListEmail(envOr("APP_URL", "http://localhost:5173"), removed, strings.Join(args[2:], " "))
		if err != nil {
			return err
		}
		if err := mailer().Send(ctx, m); err != nil {
			return fmt.Errorf("made %d private, but couldn't email its owner at %s: %w", id, removed.Email, err)
		}
		fmt.Printf("Made %d private and emailed %s.\n", id, removed.Email)
	default:
		return usage
	}
	return nil
}

func mailer() mail.SMTP {
	return mail.SMTP{
		Addr:     envOr("SMTP_ADDR", "localhost:1025"),
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     envOr("MAIL_FROM", "Tethr <sign-in@mail.tethr.tv>"),
	}
}

func serve(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, envOr("DATABASE_URL", localDatabase))
	if err != nil {
		return err
	}
	defer db.Close()

	appURL := envOr("APP_URL", "http://localhost:5173")
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		return err
	}
	origins := strings.Split(envOr("ALLOWED_ORIGINS", appURL), ",")
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          envOr("WEBAUTHN_RP_ID", "localhost"),
		RPDisplayName: "Tethr",
		RPOrigins:     origins,
	})
	if err != nil {
		return err
	}
	limits := limit.Standard(os.Getenv("CLIENT_IP_HEADER"))
	v1 := &api.Server{
		DB:             db,
		WebAuthn:       wa,
		RequireInvite:  os.Getenv("SIGNUPS") == "invite",
		Log:            log,
		AppURL:         appURL,
		AllowedOrigins: origins,
		Limits:         limits,
		Version:        version,
		Cookie: api.CookieConfig{
			Name:   envOr("SESSION_COOKIE", "tethr_session"),
			Domain: os.Getenv("COOKIE_DOMAIN"),
			Secure: strings.HasPrefix(appURL, "https://"),
		},
		ReportsTo: envOr("REPORTS_EMAIL", "reports@tethr.tv"),
		Mail:      mailer(),
	}

	go jobs.Imports{DB: db, Log: log}.Run(ctx)
	go jobs.Cleanup{DB: db, Log: log}.Run(ctx)
	go jobs.Reports{DB: db, Mail: v1.Mail, To: v1.ReportsTo, AppURL: appURL, Zone: london, Log: log}.Run(ctx)
	go jobs.Titles{DB: db, Wikidata: &wikidata.Client{UserAgent: "Tethr/" + version + " (https://github.com/tethrtv/tethr)"}, Log: log}.Run(ctx)
	media := &hooks.Server{DB: db, Log: log}
	var details *meta.Server
	if token := os.Getenv("TMDB_TOKEN"); token != "" {
		client := &tmdb.Client{Token: token}
		outlines := jobs.Outlines{DB: db, TMDB: client, Log: log}
		go outlines.Run(ctx)
		v1.FetchOrder = outlines.Order
		media.TMDB = client
		details = &meta.Server{TMDB: client, Log: log, SignedIn: v1.SignedIn}
	} else {
		log.Warn("TMDB_TOKEN isn't set, so Up next won't fill in, the website shows no names or posters, and media servers can't log episodes of shows Tethr hasn't seen yet")
	}

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           routes(v1.Handler(), &traktcompat.Server{DB: db, AppURL: appURL, Log: log, Limits: limits}, media, details),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	log.Info("listening", "addr", addr, "version", version)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-errs; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func routes(v1 http.Handler, trakt *traktcompat.Server, media *hooks.Server, details *meta.Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/", v1)
	trakt.Routes(mux)
	media.Routes(mux)
	if details != nil {
		details.Routes(mux)
	}
	api.Docs(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "tethr", "version": version})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
