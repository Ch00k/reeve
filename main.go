// reeve shows the Claude Code sessions running on this machine and stops
// them on request.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	messageCount := flag.Int("messages", 3, "how many of a session's last messages the messages window shows")
	flag.Parse()

	if *showVersion {
		fmt.Println(version())
		return
	}
	if *messageCount < 1 {
		fmt.Fprintln(os.Stderr, "-messages must be at least 1")
		os.Exit(2)
	}

	if _, err := tea.NewProgram(newModel(*messageCount)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// version is the module version Go stamped into the binary: the release tag
// for a build of a tagged commit or a `go install`, a pseudo-version otherwise.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return info.Main.Version
}
