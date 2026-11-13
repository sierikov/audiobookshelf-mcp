package agents

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"
)

// cliTimeout bounds each run of an agent's CLI, so a CLI that hangs (waiting
// for a login, say) cannot stall setup.
const cliTimeout = 60 * time.Second

// cliAgent is an agent configured through its own CLI.
type cliAgent struct {
	about
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
	// adapt, when set, turns the entry into the command the agent should run.
	adapt func(Entry) Entry
	// remove, when set, runs before add to drop an existing entry; it fails
	// when there is none, so its error is ignored.
	remove []string
	// add is the arguments that add our entry, up to the server name; the
	// name, "--", the command and its args follow.
	add []string
}

func claudeCode(env Env) Agent {
	return cliAgent{
		about: about{
			id: "claude-code", name: "Claude Code",
			restart: `Start a new Claude Code session. If you added it before with claude mcp add, ` +
				`also run "claude mcp remove audiobookshelf -s local" in that project.`,
			bin: "claude", lookPath: env.LookPath,
		},
		run:    env.Run,
		adapt:  func(e Entry) Entry { return npxThroughCmd(env.GOOS, e) },
		remove: []string{"mcp", "remove", ServerName, "-s", "user"},
		add:    []string{"mcp", "add", "--scope", "user", "--transport", "stdio"},
	}
}

func opencode(env Env) Agent {
	return cliAgent{
		about: about{
			id: "opencode", name: "OpenCode", restart: "Restart OpenCode.",
			bin: "opencode", lookPath: env.LookPath,
		},
		run: env.Run,
		// opencode mcp add replaces an existing entry itself.
		add: []string{"mcp", "add"},
	}
}

// Add runs the agent's CLI to (re)add our entry.
func (a cliAgent) Add(ctx context.Context, e Entry) (Result, error) {
	if a.adapt != nil {
		e = a.adapt(e)
	}
	if a.remove != nil {
		_, _ = a.runCLI(ctx, a.remove)
	}
	args := slices.Concat(a.add, []string{ServerName, "--", e.Command}, e.Args)
	out, err := a.runCLI(ctx, args)
	if err != nil {
		if detail := bytes.TrimSpace(out); len(detail) > 0 {
			err = fmt.Errorf("%w: %s", err, detail)
		}
		return Result{}, fmt.Errorf("%s mcp add: %w", a.bin, err)
	}
	return Result{Restart: a.restart}, nil
}

// runCLI runs the agent's binary with args, giving up after cliTimeout. A
// command that was killed for taking too long fails with "no answer", not with
// the "signal: killed" the process reports.
func (a cliAgent) runCLI(ctx context.Context, args []string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()
	out, err := a.run(runCtx, a.bin, args...)
	if err != nil && ctx.Err() == nil && runCtx.Err() != nil {
		return out, fmt.Errorf("no answer after %d seconds: %w", int(cliTimeout.Seconds()), runCtx.Err())
	}
	return out, err
}

// npxThroughCmd returns e to run through "cmd /c" when it is npx on Windows,
// where npx is a .cmd shim that an agent cannot start directly. Any other
// entry comes back as it is.
func npxThroughCmd(goos string, e Entry) Entry {
	if goos != "windows" || e.Command != "npx" {
		return e
	}
	return Entry{Command: "cmd", Args: slices.Concat([]string{"/c", "npx"}, e.Args)}
}
