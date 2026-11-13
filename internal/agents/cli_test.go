package agents

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder is a fake Env.Run that records every command. A command whose
// subcommand (the second argument, as in "mcp add") is in fail exits with an
// error and the given output.
type recorder struct {
	calls [][]string
	fail  map[string]string
}

func (r *recorder) run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(args) > 1 {
		if out, ok := r.fail[args[1]]; ok {
			return []byte(out), errors.New("exit status 1")
		}
	}
	return []byte("ok\n"), nil
}

func TestClaudeCode_RemovesThenAdds(t *testing.T) {
	wantCalls := [][]string{
		{"claude", "mcp", "remove", "audiobookshelf", "-s", "user"},
		{"claude", "mcp", "add", "--scope", "user", "--transport", "stdio", "audiobookshelf",
			"--", "npx", "-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
	}

	t.Run("success", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "claude")
		rec := &recorder{}
		env.Run = rec.run

		res, err := agentByID(t, env, "claude-code").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		assert.Equal(t, wantCalls, rec.calls)
		assert.Equal(t, Result{Restart: `Start a new Claude Code session. If you added it before with claude mcp add, ` +
			`also run "claude mcp remove audiobookshelf -s local" in that project.`}, res)
	})

	t.Run("a failing remove is ignored", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "claude")
		rec := &recorder{fail: map[string]string{"remove": "No MCP server found"}}
		env.Run = rec.run

		_, err := agentByID(t, env, "claude-code").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		assert.Equal(t, wantCalls, rec.calls)
	})

	t.Run("a failing add returns its output", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "claude")
		rec := &recorder{fail: map[string]string{"add": "Invalid transport\n"}}
		env.Run = rec.run

		_, err := agentByID(t, env, "claude-code").Add(t.Context(), npxEntry())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid transport")
		assert.Contains(t, err.Error(), "exit status 1")
	})
}

func TestClaudeCode_WrapsNPXOnWindowsOnly(t *testing.T) {
	addArgs := []string{"claude", "mcp", "add", "--scope", "user", "--transport", "stdio", "audiobookshelf", "--"}
	exe := `C:\Users\me\AppData\Local\audiobookshelf-mcp\audiobookshelf-mcp.exe`
	tests := []struct {
		name  string
		goos  string
		entry Entry
		want  []string
	}{
		{
			name:  "windows npx runs through cmd",
			goos:  "windows",
			entry: npxEntry(),
			want:  []string{"cmd", "/c", "npx", "-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
		},
		{
			name:  "windows executable is unchanged",
			goos:  "windows",
			entry: Entry{Command: exe},
			want:  []string{exe},
		},
		{
			name:  "darwin npx is unchanged",
			goos:  "darwin",
			entry: npxEntry(),
			want:  []string{"npx", "-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := fakeEnv(t, tt.goos, nil, "claude")
			rec := &recorder{}
			env.Run = rec.run
			before := Entry{Command: tt.entry.Command, Args: slices.Clone(tt.entry.Args)}

			_, err := agentByID(t, env, "claude-code").Add(t.Context(), tt.entry)

			require.NoError(t, err)
			require.Len(t, rec.calls, 2)
			assert.Equal(t, slices.Concat(addArgs, tt.want), rec.calls[1])
			assert.Equal(t, before, tt.entry, "the caller's entry is not modified")
		})
	}
}

func TestCLIAgent_EachCommandHasATimeout(t *testing.T) {
	for _, id := range []string{"claude-code", "opencode"} {
		t.Run(id, func(t *testing.T) {
			env := fakeEnv(t, "darwin", nil, "claude", "opencode")
			var deadlines []time.Time
			env.Run = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
				deadline, ok := ctx.Deadline()
				assert.True(t, ok, "the command runs without a deadline")
				deadlines = append(deadlines, deadline)
				return nil, nil
			}

			_, err := agentByID(t, env, id).Add(t.Context(), npxEntry())

			require.NoError(t, err)
			require.NotEmpty(t, deadlines)
			for _, d := range deadlines {
				assert.Positive(t, time.Until(d))
				assert.LessOrEqual(t, time.Until(d), 60*time.Second)
			}
		})
	}
}

func TestCLIAgent_ACommandThatNeverAnswersIsReportedAsSuch(t *testing.T) {
	// A killed command fails with "signal: killed", which says nothing about why.
	hang := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, errors.New("signal: killed")
	}

	t.Run("after the timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			env := fakeEnv(t, "darwin", nil, "opencode")
			env.Run = hang
			start := time.Now()

			_, err := agentByID(t, env, "opencode").Add(t.Context(), npxEntry())

			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.EqualError(t, err, "opencode mcp add: no answer after 60 seconds: context deadline exceeded")
			assert.Equal(t, cliTimeout, time.Since(start))
		})
	})

	t.Run("not when the caller gave up", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "opencode")
		env.Run = hang
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := agentByID(t, env, "opencode").Add(ctx, npxEntry())

		assert.EqualError(t, err, "opencode mcp add: signal: killed")
	})
}

func TestClaudeCode_EmptyArgs(t *testing.T) {
	env := fakeEnv(t, "darwin", nil, "claude")
	rec := &recorder{}
	env.Run = rec.run

	_, err := agentByID(t, env, "claude-code").Add(t.Context(), Entry{Command: "/opt/homebrew/bin/audiobookshelf-mcp"})

	require.NoError(t, err)
	require.Len(t, rec.calls, 2)
	assert.Equal(t, []string{"claude", "mcp", "add", "--scope", "user", "--transport", "stdio", "audiobookshelf",
		"--", "/opt/homebrew/bin/audiobookshelf-mcp"}, rec.calls[1])
}

func TestOpenCode_Adds(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "opencode")
		rec := &recorder{}
		env.Run = rec.run

		res, err := agentByID(t, env, "opencode").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		assert.Equal(t, [][]string{
			{"opencode", "mcp", "add", "audiobookshelf", "--", "npx", "-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
		}, rec.calls)
		assert.Equal(t, Result{Restart: "Restart OpenCode."}, res)
	})

	t.Run("failure", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "opencode")
		rec := &recorder{fail: map[string]string{"add": "permission denied"}}
		env.Run = rec.run

		_, err := agentByID(t, env, "opencode").Add(t.Context(), npxEntry())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})
}
