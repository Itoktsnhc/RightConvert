package main

import (
	"errors"
	"path/filepath"
	"strings"
)

// command is what a command line asks for.
type command struct {
	Kind  string // "settings", "convert", "register", "unregister"
	Rule  string
	Files []string
}

// parseArgs reads the arguments after the program name:
//
//	(none)                              open the settings window
//	convert --rule ID FILE...           convert files
//	--register | --unregister           write or remove the context menu
//
// wd resolves relative file names, as a second instance's arguments come
// from another working directory.
func parseArgs(args []string, wd string) (command, error) {
	if len(args) == 0 {
		return command{Kind: "settings"}, nil
	}
	switch args[0] {
	case "--register", "register":
		return command{Kind: "register"}, nil
	case "--unregister", "unregister":
		return command{Kind: "unregister"}, nil
	case "convert":
	default:
		return command{}, errors.New("unknown command " + args[0])
	}
	c := command{Kind: "convert"}
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--rule" && i+1 < len(rest):
			i++
			c.Rule = rest[i]
		case strings.HasPrefix(a, "--rule="):
			c.Rule = strings.TrimPrefix(a, "--rule=")
		case a == "--":
			for _, f := range rest[i+1:] {
				c.Files = append(c.Files, abs(f, wd))
			}
			i = len(rest)
		case strings.HasPrefix(a, "--"):
			return command{}, errors.New("unknown option " + a)
		default:
			c.Files = append(c.Files, abs(a, wd))
		}
	}
	if c.Rule == "" {
		return command{}, errors.New("convert needs --rule")
	}
	if len(c.Files) == 0 {
		return command{}, errors.New("convert needs at least one file")
	}
	return c, nil
}

func abs(p, wd string) string {
	if filepath.IsAbs(p) || wd == "" {
		return filepath.Clean(p)
	}
	return filepath.Join(wd, p)
}
