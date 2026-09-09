// Command valheim-ui is the Valheim server manager.
//
//	valheim-ui serve   [--config PATH]        run the web application (default)
//	valheim-ui launch  --instance ID           exec the game server (used by systemd)
//	valheim-ui admin   <create-user|reset-password|list-users> ...
//	valheim-ui migrate [--config PATH]         apply database migrations and exit
//	valheim-ui version
package main

import (
	"fmt"
	"os"
)

// Set via -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "none"
)

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "launch":
		err = runLaunch(args)
	case "admin":
		err = runAdmin(args)
	case "migrate":
		err = runMigrate(args)
	case "version":
		fmt.Printf("valheim-ui %s (%s)\n", version, commit)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

const usage = `usage: valheim-ui <serve|launch|admin|migrate|version> [flags]
  serve   [--config PATH]           run the web application (default)
  launch  --instance ID             exec the game server (used by systemd)
  admin   <subcommand> [flags]      user administration (see admin --help)
  migrate [--config PATH]           apply database migrations and exit
  version                           print version
`
