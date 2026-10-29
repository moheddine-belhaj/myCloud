package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/moheddine-belhaj/mycloud/backend/config"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/migrations"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/password"
	"github.com/moheddine-belhaj/mycloud/backend/internal/database"
)

// runCLI connects to the database named by DATABASE_URL (already set inside
// the pod), makes sure the schema is current, and runs one admin command.
//
//	kubectl -n mycloud-dev exec -i deploy/auth -- /app create-admin --email me@example.com < pw.txt
//	kubectl -n mycloud-dev exec deploy/auth -- /app create-invite --ttl 72h
func runCLI(cmd string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	// Idempotent and guarded by an advisory lock, so it is safe while the
	// server pods are running.
	if err := database.NewMigrator(pool, migrations.FS).Up(ctx); err != nil {
		return err
	}
	svc := auth.NewService(pool, password.NewHasher(password.DefaultParams, 1))

	switch cmd {
	case "create-admin":
		return createAdmin(ctx, svc, args, stdin, stdout, stderr)
	case "create-invite":
		return createInvite(ctx, svc, args, stdout, stderr)
	case "reset-2fa":
		return reset2FA(ctx, svc, args, stdout, stderr)
	}
	return fmt.Errorf("unknown command %q", cmd)
}

// createAdmin reads the password from the first line of stdin, so it never
// appears in the process list, shell history or kubectl output.
func createAdmin(ctx context.Context, svc *auth.Service, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	email := fs.String("email", "", "email address of the new admin (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" || fs.NArg() > 0 {
		return errors.New("usage: create-admin --email EMAIL (password on stdin)")
	}

	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read password from stdin: %w", err)
	}
	pw := strings.TrimRight(line, "\r\n")
	if pw == "" {
		return errors.New("no password on stdin: pipe it in, e.g. printf '%s' \"$PW\" | ... create-admin --email EMAIL")
	}

	u, err := svc.CreateAdmin(ctx, *email, pw)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created admin %s (id %s)\n", u.Email, u.ID)
	return nil
}

// createInvite prints only the code on stdout, so it can be captured:
//
//	CODE=$(kubectl -n mycloud-dev exec deploy/auth -- /app create-invite)
func createInvite(ctx context.Context, svc *auth.Service, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("create-invite", flag.ContinueOnError)
	fs.SetOutput(stderr)
	ttl := fs.Duration("ttl", auth.DefaultInviteTTL, "how long the invite stays valid (1h to 720h)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: create-invite [--ttl 168h]")
	}

	inv, err := svc.CreateInvite(ctx, nil, *ttl)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, inv.Code)
	fmt.Fprintf(stderr, "invite expires at %s\n", inv.ExpiresAt.UTC().Format(time.RFC3339))
	return nil
}

// reset2FA removes 2FA from an account whose owner lost both the phone and
// the recovery codes, and logs them out everywhere:
//
//	kubectl -n mycloud-dev exec deploy/auth -- /app reset-2fa --email mo@example.com
//
// Check who is asking (out of band) before running it.
func reset2FA(ctx context.Context, svc *auth.Service, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("reset-2fa", flag.ContinueOnError)
	fs.SetOutput(stderr)
	email := fs.String("email", "", "email address of the account (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" || fs.NArg() > 0 {
		return errors.New("usage: reset-2fa --email EMAIL")
	}
	u, err := svc.ResetTOTP(ctx, *email)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "2FA removed and sessions ended for %s (id %s)\n", u.Email, u.ID)
	return nil
}
