package main

import (
	"github.com/AsterZephyr/Scree-go-AZlearn/cmd"
	pmode "github.com/AsterZephyr/Scree-go-AZlearn/config/mode"
)

var (
	version    = "unknown"
	commitHash = "unknown"
	mode       = pmode.Dev
)

func main() {
	pmode.Set(mode)
	cmd.Run(version, commitHash)
}
