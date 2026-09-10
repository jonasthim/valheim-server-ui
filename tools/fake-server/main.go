// Command fake-server is a portable stand-in for the Valheim dedicated server,
// the Go twin of testdata/fake-server.sh for platforms without a shell
// (Windows) and for the launcher/supervisor tests. It prints the console
// lines the log parser cares about (ARCHITECTURE.md §8) and exits cleanly on
// SIGINT / Ctrl+C.
//
// FAKE_SERVER_DUMP=<file> makes it write its arguments, cwd and the launcher
// environment it received to that file, for tests.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"
)

func main() {
	name, port, world := "Fake", "2456", "Dedicated"
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-name", "-port", "-world":
			if i+1 < len(args) {
				switch args[i] {
				case "-name":
					name = args[i+1]
				case "-port":
					port = args[i+1]
				case "-world":
					world = args[i+1]
				}
				i++
			}
		case "-password":
			i++
		}
	}
	if dump := os.Getenv("FAKE_SERVER_DUMP"); dump != "" {
		wd, _ := os.Getwd()
		body := "ARGS:" + strings.Join(args, " ") + "\n" +
			"STEAMAPPID:" + os.Getenv("SteamAppId") + "\n" +
			"LDLIB:" + os.Getenv("LD_LIBRARY_PATH") + "\n" +
			"DOORSTOP:" + os.Getenv("DOORSTOP_ENABLED") + "\n" +
			"PWD:" + wd + "\n"
		_ = os.WriteFile(dump, []byte(body), 0o600) //nolint:gosec // test-only dump path chosen by the test
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, terminateSignal)

	say := func(format string, a ...any) {
		fmt.Printf("%s: %s\n", time.Now().Format("01/02/2006 15:04:05"), fmt.Sprintf(format, a...))
	}
	sleep := func(d time.Duration) bool {
		select {
		case <-stop:
			return false
		case <-time.After(d):
			return true
		}
	}

	fmt.Println("Starting server PRESS CTRL-C to exit")
	doorstop := os.Getenv("DOORSTOP_ENABLED")
	if doorstop == "" {
		doorstop = "unset"
	}
	say("Valheim version: 0.220.5 (fake) DOORSTOP_ENABLED=%s SteamAppId=%s", doorstop, envOr("SteamAppId", "unset"))
	say("Load world: %s", world)
	alive := sleep(time.Second)
	if alive {
		say("Zonesystem Start 0")
		say("DungeonDB Start 0")
		alive = sleep(time.Second)
	}
	if alive {
		say("Steam game server initialized")
		say("Game server connected")
		say("Session %q with join code 123456 and IP 203.0.113.10:%s is active with 0 player(s)", name, port)
	}
	players := 0
	for i := 1; alive; i++ {
		if !sleep(time.Second) {
			break
		}
		switch i {
		case 5:
			say("Got connection SteamID 76561198000000001")
			say("Got handshake from client 76561198000000001")
			say("Got character ZDOID from Bjorn : 12345:1")
			players = 1
		case 25:
			say("Closing socket 76561198000000001")
			players = 0
		}
		if i%10 == 0 {
			say("Session %q with join code 123456 and IP 203.0.113.10:%s is active with %d player(s)", name, port, players)
		}
		if i%30 == 0 {
			say("World saved ( 42.1ms )")
		}
	}
	say("OnApplicationQuit")
	say("World saved ( 38.7ms )")
	say("Net scene destroyed")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
