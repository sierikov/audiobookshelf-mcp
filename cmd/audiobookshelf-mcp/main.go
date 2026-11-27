// Package main handles the command-line interface for the Audiobookshelf MCP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/term"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/agents"
	"github.com/sierikov/audiobookshelf-mcp/internal/buildinfo"
	"github.com/sierikov/audiobookshelf-mcp/internal/config"
	"github.com/sierikov/audiobookshelf-mcp/internal/gemini"
	"github.com/sierikov/audiobookshelf-mcp/internal/service"
	"github.com/sierikov/audiobookshelf-mcp/internal/setup"
	"github.com/sierikov/audiobookshelf-mcp/internal/tools"
	"github.com/sierikov/audiobookshelf-mcp/internal/update"
)

// Exit codes of the command line.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
	// exitInterrupted is the shell's code for a process stopped by Ctrl-C
	// (128 + SIGINT).
	exitInterrupted = 130
)

// updateFetchTimeout bounds the request that looks up the latest release.
const updateFetchTimeout = 5 * time.Second

// notConfigured is the error of a server that has neither a settings file nor
// the environment variables to tell it where Audiobookshelf is.
const notConfigured = `audiobookshelf-mcp is not configured: run "audiobookshelf-mcp setup" ` +
	`(with npx: "npx -y @sierikov/audiobookshelf-mcp setup"), or set AUDIOBOOKSHELF_URL and AUDIOBOOKSHELF_TOKEN`

// usage is the help text. It starts with "usage" in lower case, as Go's flag
// package does.
const usage = `usage: audiobookshelf-mcp [command]

With no command, audiobookshelf-mcp serves MCP on standard input and output,
which is how an AI agent starts it.

commands:
  setup     connect to Audiobookshelf and add this server to your AI agents
            (run "audiobookshelf-mcp setup -h" for its options)
  version   print the version and say when a newer release is out
  help      show this text
`

// setupUsage introduces the flags of the setup command.
const setupUsage = `usage: audiobookshelf-mcp setup [flags]

Checks your Audiobookshelf URL and token, saves them, and adds this server to
the AI agents found on this machine. It asks for whatever the flags leave out.
With covers on, it also asks for a Gemini API key, which pays for made covers
(about $0.07 to $0.16 a cover). No flag takes the key: a flag shows in the
process list.

flags:
`

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run executes one command line and returns the process exit code: 0 for
// success, 1 for an error and 2 for a command line it does not understand.
//
// stdin and stdout feed the setup prompts and receive its output; the commands
// print their errors to stderr. With no arguments it serves MCP, which always
// uses the process's own standard input and output: nothing else writes to
// stdout then, because that stream belongs to the protocol.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return finish(stderr, serve(ctx))
	}
	switch args[0] {
	case "setup":
		return runSetup(ctx, args[1:], stdin, stdout, stderr)
	case "version", "--version":
		say(stdout, "audiobookshelf-mcp %s\n", buildinfo.Version())
		// The notice goes to stderr, so a script that reads the version from
		// stdout never gets it mixed in.
		printUpdateNotice(ctx, stderr)
		return exitOK
	case "help", "-h", "--help":
		say(stdout, "%s", usage)
		return exitOK
	default:
		say(stderr, "audiobookshelf-mcp: unknown command %q\n\n%s", args[0], usage)
		return exitUsage
	}
}

// say writes formatted text to w. A failed write is ignored: there is nowhere
// left to report it.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// finish prints err, when there is one, as "error: ..." on stderr and returns
// the exit code.
func finish(stderr io.Writer, err error) int {
	if err == nil {
		return exitOK
	}
	say(stderr, "error: %v\n", err)
	return exitError
}

// serve runs the MCP server on standard input and output until the client
// disconnects or the process is asked to stop.
func serve(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.URL == "" || cfg.Token == "" {
		return notConfiguredError()
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	ver := buildinfo.Version()
	checker, _ := newChecker(ver, os.Getenv) // nil when the update check is off
	client := abs.NewClient(cfg.URL, cfg.Token)
	server := tools.NewServer(newServices(cfg, client),
		tools.Options{Version: ver, Toolsets: cfg.Toolsets, Instructions: updateInstructions(checker)})

	if checker != nil {
		// The answer is only for the next start, so its error has nowhere to go.
		go func() { _ = checker.Refresh(ctx) }()
	}

	// Being told to stop is how an agent normally ends the server, not a failure.
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// newServices builds the services the tools call, backed by client. The tools
// that make covers cost money, so their service exists only when cfg has a
// Gemini API key. It is built inside the check and never assigned from a
// variable: a nil *service.CoverArt in the interface would not be nil, and
// would offer the paid tools without a key.
func newServices(cfg config.Config, client *abs.Client) tools.Services {
	svcs := tools.Services{
		Libraries: service.NewLibraries(client),
		Items:     service.NewItems(client),
		Playback:  service.NewPlayback(client),
		Browse:    service.NewBrowse(client),
		Chapters:  service.NewChapters(client),
		Metadata:  service.NewMetadata(client),
		Covers:    service.NewCovers(client, service.NewImageFetcher(), cfg.URL),
	}
	if cfg.GeminiKey != "" {
		svcs.CoverArt = service.NewCoverArt(client, gemini.NewClient(gemini.DefaultBaseURL, cfg.GeminiKey))
	}
	return svcs
}

// notConfiguredError is the error for a server without settings. When the
// settings location could not even be worked out (no home directory), it says
// so, because that is why a settings file could not have been found.
func notConfiguredError() error {
	if _, err := config.Path(); err != nil {
		return fmt.Errorf("%s (settings location unknown: %w)", notConfigured, err)
	}
	return errors.New(notConfigured)
}

// loadConfig reads the settings file and applies the AUDIOBOOKSHELF_*
// environment variables on top. A missing settings file is not an error, and
// neither is a settings location that cannot be determined (no home
// directory): the environment variables alone can still configure the server.
func loadConfig() (config.Config, error) {
	var cfg config.Config
	if path, err := config.Path(); err == nil {
		cfg, err = config.Load(path)
		if err != nil {
			return config.Config{}, err
		}
	}
	return cfg.WithEnv(os.Getenv), nil
}

// runSetup runs the setup command and returns its exit code.
func runSetup(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := newSetupFlagSet()
	opts, err := parseSetupFlags(flags, args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		printSetupUsage(stdout, flags)
		return exitOK
	case err != nil:
		say(stderr, "error: %v\n\n", err)
		printSetupUsage(stderr, flags)
		return exitUsage
	}

	deps, err := setupDeps(stdin, stderr)
	if err != nil {
		return finish(stderr, err)
	}
	if err := setup.Run(ctx, stdin, stdout, opts, deps); err != nil {
		return finish(stderr, err)
	}
	printUpdateNotice(ctx, stdout)
	return exitOK
}

// newSetupFlagSet returns the flag set of the setup command. It prints
// nothing itself: runSetup reports parse errors and the usage.
func newSetupFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	return flags
}

// printSetupUsage prints the usage of the setup command, with the flags that
// parseSetupFlags defined on flags.
func printSetupUsage(w io.Writer, flags *flag.FlagSet) {
	say(w, "%s", setupUsage)
	flags.SetOutput(w)
	flags.PrintDefaults()
}

// parseSetupFlags defines the flags of the setup command on flags and parses
// args into the options for [setup.Run].
func parseSetupFlags(flags *flag.FlagSet, args []string) (setup.Options, error) {
	var (
		opts     setup.Options
		toolsets string
		agentIDs string
	)
	flags.StringVar(&opts.URL, "url", "", "Audiobookshelf `URL`")
	flags.StringVar(&opts.Token, "token", "",
		"API `token`; other programs on this machine can see it in the process list, so the prompt is safer")
	flags.StringVar(&toolsets, "toolsets", "",
		"toolsets to use, as a comma-separated `list`. It replaces the defaults (libraries,items,playback): "+
			"for example libraries,items,playback,browse,chapters,metadata,covers, or all. An empty value restores the defaults.")
	flags.StringVar(&agentIDs, "agents", "",
		"agents to add this server to, as a comma-separated `list` of IDs, or none")
	flags.BoolVar(&opts.Yes, "yes", false, "never ask: use the flags, the saved settings and the defaults")

	if err := flags.Parse(args); err != nil {
		return setup.Options{}, err
	}
	if flags.NArg() > 0 {
		// The value is never shown: a misplaced token may be among them.
		return setup.Options{}, fmt.Errorf("setup takes only flags (got %d extra argument(s))", flags.NArg())
	}
	// An empty --toolsets still has to count as given, so it can switch the
	// saved toolsets off.
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "toolsets" {
			opts.ToolsetsSet = true
		}
	})
	opts.Toolsets = config.SplitList(toolsets)
	if err := checkToolsets(opts.Toolsets); err != nil {
		return setup.Options{}, err
	}
	opts.Agents = config.SplitList(agentIDs)
	return opts, nil
}

// checkToolsets rejects the first name that is neither a toolset of the
// server nor "all", so that a typo fails here instead of silently turning
// nothing on.
func checkToolsets(names []string) error {
	known := make([]string, 0, len(tools.AllToolsetMetas())+1)
	for _, meta := range tools.AllToolsetMetas() {
		known = append(known, string(meta.ID))
	}
	known = append(known, "all")
	for _, name := range names {
		if !slices.Contains(known, name) {
			return fmt.Errorf("unknown toolset %q; known: %s", name, strings.Join(known, ", "))
		}
	}
	return nil
}

// setupDeps wires the setup flow to this machine: the real settings file, the
// real agents, Audiobookshelf and Google, the command that agents will run,
// and the process environment. stderr receives the line break when Ctrl-C
// stops a hidden prompt.
func setupDeps(stdin io.Reader, stderr io.Writer) (setup.Deps, error) {
	cfgPath, err := config.Path()
	if err != nil {
		return setup.Deps{}, fmt.Errorf("finding the settings location: %w", err)
	}
	env, err := agents.DefaultEnv()
	if err != nil {
		return setup.Deps{}, err
	}
	exe, method, err := detectInstall()
	if err != nil {
		return setup.Deps{}, fmt.Errorf("locating this program: %w", err)
	}
	return setup.Deps{
		ConfigPath:     cfgPath,
		Agents:         agents.All(env),
		Verify:         setup.VerifyWith,
		CheckGeminiKey: geminiKeyChecker(gemini.DefaultBaseURL),
		Entry:          setup.EntryFor(method, exe, buildinfo.Version()),
		ReadSecret:     secretReader(stdin, stderr),
		Getenv:         os.Getenv,
	}, nil
}

// geminiKeyChecker returns the function setup uses to check a Gemini API key
// against the Gemini API at baseURL. The check is free.
func geminiKeyChecker(baseURL string) func(ctx context.Context, key string) error {
	return func(ctx context.Context, key string) error {
		return gemini.NewClient(baseURL, key).CheckKey(ctx)
	}
}

// secretReader returns the function that reads the token, and the Gemini key,
// without echoing them, or nil when stdin is not a terminal. A pipe or a file
// has nothing to hide, and setup reads them as plain lines from the same
// buffered reader as its other answers; reading around that buffer would lose
// input.
//
// The terminal does not echo while a secret is read. Ctrl-C at that prompt
// would leave it so, so the reader catches the interrupt itself: it restores
// the terminal, ends the prompt line on stderr and exits with
// exitInterrupted.
func secretReader(stdin io.Reader, stderr io.Writer) func() (string, error) {
	f, ok := stdin.(*os.File)
	if !ok {
		return nil
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return nil
	}
	return func() (string, error) {
		state, err := term.GetState(fd)
		if err != nil {
			return "", err
		}
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		done := make(chan struct{})
		go func() {
			select {
			case <-interrupts:
				_ = term.Restore(fd, state)
				say(stderr, "\n")
				os.Exit(exitInterrupted)
			case <-done:
			}
		}()
		token, err := term.ReadPassword(fd)
		signal.Stop(interrupts)
		close(done)
		return string(token), err
	}
}

// printUpdateNotice tells the user, on out, that a newer release exists, as
// far as the running build and the process environment allow: nothing is
// printed for a dev build or when the check is switched off.
func printUpdateNotice(ctx context.Context, out io.Writer) {
	if checker, ok := newChecker(buildinfo.Version(), os.Getenv); ok {
		reportUpdate(ctx, out, checker)
	}
}

// reportUpdate prints the CLI notice on out when checker finds a newer
// release. It refreshes the release cache if it is older than a day, instead
// of leaving that to the next server start, and gives up on the lookup after
// updateFetchTimeout, which the HTTP client of newChecker enforces. Nothing is
// printed when the lookup fails and the cache holds nothing newer, or when
// nothing is newer.
func reportUpdate(ctx context.Context, out io.Writer, checker *update.Checker) {
	// A failed lookup only means no fresh answer: the cache may still hold one.
	_ = checker.Refresh(ctx)
	if latest, method, ok := pendingUpdate(checker); ok {
		say(out, "%s\n", update.CLINotice(latest, checker.Current, method))
	}
}

// updateInstructions returns the server instructions that tell the agent about
// a newer release, or "" when there is none to report or checker is nil (the
// update check is off). It reads only the cached result of an earlier check, so
// it never waits on the network; serve refreshes that cache in the background
// for the next start. Failures along the way mean "no notice" and never stop
// the server.
func updateInstructions(checker *update.Checker) string {
	if checker == nil {
		return ""
	}
	if latest, method, ok := pendingUpdate(checker); ok {
		return update.AgentNotice(latest, checker.Current, method)
	}
	return ""
}

// newChecker returns the update checker for the running version, with getenv
// as the environment. It is false when the check is off: a build that is not a
// release, an opt-out through the environment, or no cache directory to keep
// the result in.
func newChecker(version string, getenv func(string) string) (*update.Checker, bool) {
	if !update.Enabled(version, getenv) {
		return nil, false
	}
	cachePath, err := update.DefaultCachePath()
	if err != nil {
		return nil, false
	}
	return &update.Checker{
		Current:   version,
		CachePath: cachePath,
		Now:       time.Now,
		Fetch: func(ctx context.Context) (string, error) {
			client := &http.Client{Timeout: updateFetchTimeout}
			return update.FetchLatest(ctx, client, update.ReleasesURL, version)
		},
	}, true
}

// pendingUpdate returns the newer release that the checker's cache records,
// and how this binary was installed, which decides the upgrade command. It is
// false when there is no newer release or the install method cannot be told.
func pendingUpdate(checker *update.Checker) (latest string, method update.Method, ok bool) {
	latest, ok = checker.Latest()
	if !ok || !update.Newer(latest, checker.Current) {
		return "", "", false
	}
	_, method, err := detectInstall()
	if err != nil {
		return "", "", false
	}
	return latest, method, true
}

// detectInstall locates the running binary, with symlinks resolved, and works
// out how it was installed. It fails when the executable cannot be located.
func detectInstall() (string, update.Method, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", "", err
	}
	home, _ := os.UserHomeDir() // without a home directory, GOBIN or GOPATH may still say
	return exe, update.DetectMethod(exe, update.GoBin(os.Getenv, home)), nil
}
