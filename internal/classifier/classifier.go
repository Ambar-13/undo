package classifier

import (
	"regexp"
	"strings"

	"github.com/Ambar13/undo/internal/store"
)

var (
	reRm       = regexp.MustCompile(`^rm\s+`)
	reMv       = regexp.MustCompile(`^mv\s+`)
	reCp       = regexp.MustCompile(`^cp\s+`)
	reRedirect = regexp.MustCompile(`>\s*(\S+)`)
	reTruncate = regexp.MustCompile(`^truncate\s+`)
	reShred    = regexp.MustCompile(`^shred\s+`)
	reGitClean = regexp.MustCompile(`^git\s+clean\s+`)
)

// Classify returns the operation type and affected source paths for a shell command.
// For moves, only the source path is returned (destination is new location, not at risk).
func Classify(cmd string) (store.OpType, []string) {
	cmd = strings.TrimSpace(cmd)

	if reRm.MatchString(cmd) || reShred.MatchString(cmd) {
		return store.OpDelete, extractNonFlagArgs(cmd, false)
	}
	if reGitClean.MatchString(cmd) {
		return store.OpDelete, nil
	}
	if reMv.MatchString(cmd) {
		args := extractNonFlagArgs(cmd, false)
		if len(args) >= 2 {
			return store.OpMove, args[:len(args)-1] // source paths only
		}
		return store.OpMove, args
	}
	if reCp.MatchString(cmd) {
		args := extractNonFlagArgs(cmd, false)
		if len(args) >= 2 {
			return store.OpOverwrite, args[len(args)-1:] // destination only
		}
		return store.OpOverwrite, args
	}
	if m := reRedirect.FindStringSubmatch(cmd); m != nil {
		return store.OpOverwrite, []string{m[1]}
	}
	if reTruncate.MatchString(cmd) {
		// truncate flags like -s take a value argument; skip flag values too
		return store.OpOverwrite, extractNonFlagArgs(cmd, true)
	}

	return store.OpUnknown, nil
}

// extractNonFlagArgs extracts positional (non-flag) arguments from a command,
// skipping the command name itself. If skipFlagValues is true, arguments
// immediately following a short flag (e.g. -s) are also skipped.
func extractNonFlagArgs(cmd string, skipFlagValues bool) []string {
	parts := strings.Fields(cmd)
	var out []string
	skipNext := true // skip the command name itself
	for _, p := range parts {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(p, "-") {
			if skipFlagValues {
				skipNext = true // next token is the flag's value
			}
			continue
		}
		out = append(out, p)
	}
	return out
}
