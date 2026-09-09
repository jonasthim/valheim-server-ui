package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/auth"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// runAdmin provides break-glass user administration:
//
//	valheim-ui admin create-user --username U --role admin [--password P] [--config PATH]
//	valheim-ui admin reset-password --username U [--password P] [--config PATH]
//	valheim-ui admin list-users [--config PATH]
const adminUsage = `usage: valheim-ui admin <create-user|reset-password|list-users> [flags]
  create-user     --username U --role <viewer|operator|admin> [--password P] [--config PATH]
  reset-password  --username U [--password P] [--config PATH]
  list-users      [--config PATH]

If --password is omitted, it is read from a single line on stdin.
`

func runAdmin(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, adminUsage)
		return errors.New("admin: missing subcommand")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create-user":
		return runAdminCreateUser(rest)
	case "reset-password":
		return runAdminResetPassword(rest)
	case "list-users":
		return runAdminListUsers(rest)
	case "help", "-h", "--help":
		fmt.Print(adminUsage)
		return nil
	default:
		fmt.Fprint(os.Stderr, adminUsage)
		return fmt.Errorf("admin: unknown subcommand %q", sub)
	}
}

// openAdminDB loads the config and opens (and migrates) the database, the
// same way `serve` and `migrate` do.
func openAdminDB(cfgPath string) (*sql.DB, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	return db.Open(context.Background(), cfg.DBPath())
}

// promptPassword reads a single line from stdin when --password was omitted.
func promptPassword(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func runAdminCreateUser(args []string) error {
	fs := flag.NewFlagSet("admin create-user", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("VALHEIM_UI_CONFIG", config.DefaultPath), "config file")
	username := fs.String("username", "", "username (required)")
	role := fs.String("role", "", "role: viewer|operator|admin (required)")
	password := fs.String("password", "", "password (omit to be prompted on stdin)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	uname := strings.ToLower(strings.TrimSpace(*username))
	if uname == "" {
		return errors.New("create-user: --username is required")
	}
	r := domain.Role(*role)
	if !r.Valid() {
		return errors.New("create-user: --role must be one of viewer, operator, admin")
	}
	pw := *password
	if pw == "" {
		var err error
		pw, err = promptPassword("Password: ")
		if err != nil {
			return err
		}
	}
	if len(pw) < auth.MinPasswordLength {
		return fmt.Errorf("create-user: password must be at least %d characters", auth.MinPasswordLength)
	}

	sqldb, err := openAdminDB(*cfgPath)
	if err != nil {
		return err
	}
	defer sqldb.Close()

	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	usr, err := db.NewUsers(sqldb).Create(context.Background(), db.NewUser{
		Username: uname, Role: r, PasswordHash: &hash,
	})
	if err != nil {
		return err
	}
	fmt.Printf("created user %q (id=%d, role=%s)\n", usr.Username, usr.ID, usr.Role)
	return nil
}

func runAdminResetPassword(args []string) error {
	fs := flag.NewFlagSet("admin reset-password", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("VALHEIM_UI_CONFIG", config.DefaultPath), "config file")
	username := fs.String("username", "", "username (required)")
	password := fs.String("password", "", "new password (omit to be prompted on stdin)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	uname := strings.ToLower(strings.TrimSpace(*username))
	if uname == "" {
		return errors.New("reset-password: --username is required")
	}
	pw := *password
	if pw == "" {
		var err error
		pw, err = promptPassword("New password: ")
		if err != nil {
			return err
		}
	}
	if len(pw) < auth.MinPasswordLength {
		return fmt.Errorf("reset-password: password must be at least %d characters", auth.MinPasswordLength)
	}

	sqldb, err := openAdminDB(*cfgPath)
	if err != nil {
		return err
	}
	defer sqldb.Close()

	users := db.NewUsers(sqldb)
	usr, err := users.GetByUsername(context.Background(), uname)
	if err != nil {
		return fmt.Errorf("reset-password: %w", err)
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if err := users.SetPasswordHash(context.Background(), usr.ID, &hash); err != nil {
		return err
	}
	fmt.Printf("password reset for user %q (id=%d)\n", usr.Username, usr.ID)
	return nil
}

func runAdminListUsers(args []string) error {
	fs := flag.NewFlagSet("admin list-users", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("VALHEIM_UI_CONFIG", config.DefaultPath), "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sqldb, err := openAdminDB(*cfgPath)
	if err != nil {
		return err
	}
	defer sqldb.Close()

	users, err := db.NewUsers(sqldb).List(context.Background())
	if err != nil {
		return err
	}
	if len(users) == 0 {
		fmt.Println("no users")
		return nil
	}
	fmt.Printf("%-5s %-24s %-10s %-9s %-9s %s\n", "ID", "USERNAME", "ROLE", "DISABLED", "PASSWORD", "CREATED_AT")
	for _, u := range users {
		fmt.Printf("%-5d %-24s %-10s %-9t %-9t %s\n", u.ID, u.Username, u.Role, u.Disabled, u.HasPassword, u.CreatedAt.Format("2006-01-02T15:04:05Z"))
	}
	return nil
}
