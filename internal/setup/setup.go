// Package setup is the flow behind "audiobookshelf-mcp setup". It checks the
// Audiobookshelf URL and token, saves them to the settings file once, and adds
// this server to every AI agent found on the machine.
package setup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/sierikov/audiobookshelf-mcp/internal/agents"
	"github.com/sierikov/audiobookshelf-mcp/internal/config"
)

// maxAttempts is how many times an interactive run asks for the URL and token
// before it gives up on a connection that keeps failing.
const maxAttempts = 3

// noAgents is the --agents value, and the answer to the agent prompt, that
// skips adding the server to any agent.
const noAgents = "none"

// editingToolsets are the toolsets that write to the library. "all" includes
// them.
var editingToolsets = []string{"all", "chapters", "metadata"}

// errInputEnded is the error when the input runs out before every question is
// answered. Running out is not the same as pressing Enter: only --yes takes
// the defaults without answers.
var errInputEnded = errors.New("input ended before setup finished; " +
	"pass --yes to accept the defaults, and --agents to choose which agents to change")

// Account is what Audiobookshelf says about the token that was checked.
type Account struct {
	// Username is the user the token belongs to.
	Username string
	// Libraries is the number of libraries that user can see.
	Libraries int
	// CanUpdate reports whether the user may edit library items.
	CanUpdate bool
}

// Options are the command-line choices that decide which questions Run asks.
type Options struct {
	// URL and Token come from flags. "" means ask, or keep the saved value.
	URL, Token string
	// Toolsets is the value of --toolsets.
	Toolsets []string
	// ToolsetsSet is true when --toolsets was given, even if empty.
	ToolsetsSet bool
	// Agents is the value of --agents: agent IDs, or "none". Empty means ask.
	Agents []string
	// Yes is --yes: never prompt, take the defaults.
	Yes bool
}

// Deps is everything Run reaches outside itself, so tests can fake it.
type Deps struct {
	// ConfigPath is the settings file to read and write.
	ConfigPath string
	// Agents are all the agents this server can be added to, detected or not.
	Agents []agents.Agent
	// Verify checks a URL and token against Audiobookshelf. See [VerifyWith].
	Verify func(ctx context.Context, url, token string) (Account, error)
	// Entry is the command every agent is told to run.
	Entry agents.Entry
	// ReadSecret reads the token without echoing it. Run ends the prompt line
	// itself once it returns. Nil reads a plain line from the input instead.
	ReadSecret func() (string, error)
	// Getenv reads an environment variable. Run uses it to warn when
	// XDG_CONFIG_HOME moves the settings file away from where apps started
	// outside the shell will look. Nil skips that warning.
	Getenv func(string) string
}

// Run asks for what the flags in opts leave open, checks the connection, saves
// the settings and adds the server to the chosen agents. Prompts and results
// go to out and answers are read from in. The token is never written to out.
//
// Settings that are already saved are the defaults: pressing Enter at a prompt
// keeps them.
func Run(ctx context.Context, in io.Reader, out io.Writer, opts Options, deps Deps) error {
	s := &session{in: bufio.NewReader(in), out: out, opts: opts, deps: deps}

	// A typo in --agents is reported before anything is asked or checked.
	if err := s.checkAgentIDs(); err != nil {
		return err
	}
	saved, err := config.Load(deps.ConfigPath)
	if err != nil {
		return err
	}
	cfg, acct, err := s.connect(ctx, saved)
	if err != nil {
		return err
	}
	if cfg.Toolsets, err = s.askToolsets(saved.Toolsets); err != nil {
		return err
	}
	s.warnWithoutUpdate(acct, cfg.Toolsets)

	if err := config.Save(deps.ConfigPath, cfg); err != nil {
		return err
	}
	s.printf("Saved settings to %s.\n", deps.ConfigPath)
	s.warnXDGConfigHome()

	chosen, err := s.chooseAgents()
	if err != nil {
		return err
	}
	return s.addAll(ctx, chosen)
}

// session is one run of setup: the reader the answers come from, the writer
// everything is printed to, and the choices that steer the flow.
type session struct {
	in   *bufio.Reader
	out  io.Writer
	opts Options
	deps Deps
}

// printf writes to the output. A failed write is ignored: there is nowhere
// left to report it, and setup's real work does not depend on it.
func (s *session) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(s.out, format, args...)
}

// readLine reads one line of input without its line ending. A last line with
// no line ending still counts as an answer; input that has already ended gives
// errInputEnded.
func (s *session) readLine() (string, error) {
	line, err := s.in.ReadString('\n')
	if errors.Is(err, io.EOF) && line == "" {
		return "", errInputEnded
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading your answer: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// ask prints prompt and reads the answer.
func (s *session) ask(prompt string) (string, error) {
	s.printf("%s", prompt)
	return s.readLine()
}

// connect gets the URL and token, from flags, prompts or the saved settings,
// and checks them. An interactive run that fails asks again, up to
// maxAttempts times in all; a --yes run fails at once. On a retry, Enter at
// the URL prompt keeps the URL just tried, which the prompt shows, and Enter
// at the token prompt keeps the saved token, never the one that just failed.
// The returned settings hold the URL and token only.
func (s *session) connect(ctx context.Context, saved config.Config) (config.Config, Account, error) {
	cur := config.Config{URL: normalizeURL(saved.URL)}
	flagURL, flagToken := s.opts.URL, s.opts.Token
	for attempt := 1; ; attempt++ {
		var err error
		if cur.URL, err = s.askURL(flagURL, cur.URL); err != nil {
			return config.Config{}, Account{}, err
		}
		if cur.Token, err = s.askToken(flagToken, saved.Token); err != nil {
			return config.Config{}, Account{}, err
		}

		acct, err := s.deps.Verify(ctx, cur.URL, cur.Token)
		if err == nil {
			s.printf("Connected to %s as %s, %s.\n", cur.URL, acct.Username, countLibraries(acct.Libraries))
			return cur, acct, nil
		}
		err = withoutToken(err, cur.Token)
		if s.opts.Yes {
			return config.Config{}, Account{}, fmt.Errorf("could not connect: %w", err)
		}
		s.printf("Could not connect: %v\n", err)
		if attempt == maxAttempts {
			// The reason was printed just above.
			return config.Config{}, Account{}, fmt.Errorf("could not connect after %d attempts", attempt)
		}
		// Whatever the flags said has been tried: ask from here on.
		flagURL, flagToken = "", ""
	}
}

// askURL returns the URL: the flag, else the answer to the prompt, else def.
// A --yes run never prompts.
func (s *session) askURL(flag, def string) (string, error) {
	v := flag
	if v == "" && !s.opts.Yes {
		prompt := "Audiobookshelf URL: "
		if def != "" {
			prompt = "Audiobookshelf URL [" + def + "]: "
		}
		var err error
		if v, err = s.ask(prompt); err != nil {
			return "", err
		}
	}
	v = normalizeURL(v)
	if v == "" {
		v = def
	}
	if v == "" {
		return "", errors.New("an Audiobookshelf URL is required")
	}
	if lower := strings.ToLower(v); !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", errors.New("the URL must start with http:// or https://")
	}
	return v, nil
}

// askToken returns the API token: the flag, else the hidden answer to the
// prompt, else def. A --yes run never prompts. The token is never printed.
func (s *session) askToken(flag, def string) (string, error) {
	v := flag
	if v == "" && !s.opts.Yes {
		var err error
		if v, err = s.readToken(def != ""); err != nil {
			return "", err
		}
	}
	if v = strings.TrimSpace(v); v == "" {
		v = def
	}
	if v == "" {
		return "", errors.New("an API token is required")
	}
	return v, nil
}

// readToken prompts for the token and reads it with ReadSecret when that is
// set, so the terminal does not echo it.
func (s *session) readToken(haveSaved bool) (string, error) {
	prompt := "API token (Settings → Users → your user → API Token)"
	if haveSaved {
		prompt += ", Enter keeps the saved one"
	}
	prompt += ": "
	if s.deps.ReadSecret == nil {
		return s.ask(prompt)
	}
	s.printf("%s", prompt)
	token, err := s.deps.ReadSecret()
	// The Enter key was not echoed, so end the prompt line here.
	s.printf("\n")
	if err != nil {
		return "", fmt.Errorf("reading the token: %w", err)
	}
	return token, nil
}

// askToolsets returns the toolsets to save: the flag, else the saved ones for
// a --yes run, else the answer to the prompt. Enter keeps the saved toolsets,
// so setting up again to change the token never switches editing off.
func (s *session) askToolsets(saved []string) ([]string, error) {
	if s.opts.ToolsetsSet {
		return s.opts.Toolsets, nil
	}
	if s.opts.Yes {
		return saved, nil
	}
	choices := "[y/N]"
	if editsLibrary(saved) {
		choices = "[Y/n]"
	}
	answer, err := s.ask("Turn on the tools that edit your library (chapters, genres, tags, descriptions)? " + choices + ": ")
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(answer) {
	case "":
		return saved, nil
	case "y", "yes":
		// These are the server's toolset IDs, with the editing ones on; setup
		// cannot import the tools package to ask for them.
		return []string{"libraries", "items", "playback", "chapters", "metadata"}, nil
	case "n", "no":
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected answer %q: type y or n", answer)
	}
}

// warnXDGConfigHome says so when XDG_CONFIG_HOME puts the settings file
// somewhere other than $HOME/.config. Apps started outside the user's shell,
// such as desktop apps, may not have that variable and would look in the
// default place instead. Windows does not use XDG_CONFIG_HOME, and without
// Getenv or a home directory there is nothing to compare.
func (s *session) warnXDGConfigHome() {
	if runtime.GOOS == "windows" || s.deps.Getenv == nil {
		return
	}
	xdg, home := s.deps.Getenv("XDG_CONFIG_HOME"), s.deps.Getenv("HOME")
	if xdg == "" || home == "" {
		return
	}
	def := filepath.Join(home, ".config")
	if filepath.Clean(xdg) == def {
		return
	}
	s.printf("Note: settings are under XDG_CONFIG_HOME (%s). Apps started outside your shell, "+
		"such as Claude Desktop or Cursor, may not see that variable and will look in %s instead. "+
		"If one says the server is not configured, unset XDG_CONFIG_HOME and run setup again.\n",
		xdg, config.PathIn(def))
}

// warnWithoutUpdate says so when editing tools are on but the user may not
// edit anything in Audiobookshelf.
func (s *session) warnWithoutUpdate(acct Account, toolsets []string) {
	if editsLibrary(toolsets) && !acct.CanUpdate {
		s.printf("Note: %s has no Update permission in Audiobookshelf, so the editing tools will fail until an admin grants it.\n", acct.Username)
	}
}

// chooseAgents decides which agents to add the server to: the ones named by
// --agents; else, when none is found, nobody (with a hint for adding it by
// hand); else everything found for a --yes run; else the ones picked at the
// prompt.
func (s *session) chooseAgents() ([]agents.Agent, error) {
	var detected []agents.Agent
	for _, a := range s.deps.Agents {
		if a.Detect() {
			detected = append(detected, a)
		}
	}
	switch {
	case len(s.opts.Agents) > 0:
		return s.agentsByID(detected)
	case len(detected) == 0:
		s.printManualEntry()
		return nil, nil
	case s.opts.Yes:
		return detected, nil
	default:
		return s.pickAgents(detected)
	}
}

// checkAgentIDs fails on an --agents ID that names no agent this server
// knows. Whether the agent is installed is checked later, by agentsByID.
func (s *session) checkAgentIDs() error {
	if s.agentsNone() {
		return nil
	}
	for _, id := range s.opts.Agents {
		if _, ok := s.agentByID(id); !ok {
			return s.unknownAgent(id)
		}
	}
	return nil
}

// agentsNone reports whether --agents is "none".
func (s *session) agentsNone() bool {
	return len(s.opts.Agents) == 1 && s.opts.Agents[0] == noAgents
}

func (s *session) unknownAgent(id string) error {
	return fmt.Errorf("unknown agent %q; known: %s", id, strings.Join(s.agentIDs(), ", "))
}

// agentsByID resolves the --agents IDs. Every ID must name an agent that is
// installed, or nothing is chosen.
func (s *session) agentsByID(detected []agents.Agent) ([]agents.Agent, error) {
	if s.agentsNone() {
		return nil, nil
	}
	var chosen []agents.Agent
	for _, id := range s.opts.Agents {
		a, _ := s.agentByID(id) // checkAgentIDs has rejected unknown IDs
		if !slices.ContainsFunc(detected, func(d agents.Agent) bool { return d.ID() == id }) {
			return nil, fmt.Errorf("%s was not found on this machine", a.Name())
		}
		if !slices.ContainsFunc(chosen, func(c agents.Agent) bool { return c.ID() == id }) {
			chosen = append(chosen, a)
		}
	}
	return chosen, nil
}

func (s *session) agentByID(id string) (agents.Agent, bool) {
	i := slices.IndexFunc(s.deps.Agents, func(a agents.Agent) bool { return a.ID() == id })
	if i < 0 {
		return nil, false
	}
	return s.deps.Agents[i], true
}

func (s *session) agentIDs() []string {
	ids := make([]string, len(s.deps.Agents))
	for i, a := range s.deps.Agents {
		ids[i] = a.ID()
	}
	return ids
}

// pickAgents lists the detected agents and reads which of them to add the
// server to.
func (s *session) pickAgents(detected []agents.Agent) ([]agents.Agent, error) {
	for i, a := range detected {
		s.printf("  %d. %s\n", i+1, a.Name())
	}
	answer, err := s.ask(`Add audiobookshelf to all of these? Press Enter for all, or type numbers (e.g. 1,3), or "none": `)
	if err != nil {
		return nil, err
	}
	return parseChoice(answer, detected)
}

// parseChoice turns the answer to the agent prompt into agents: Enter picks
// all of them, "none" picks nobody, and numbers (split by commas or spaces)
// pick from the list, each at most once.
func parseChoice(answer string, detected []agents.Agent) ([]agents.Agent, error) {
	switch strings.ToLower(answer) {
	case "":
		return detected, nil
	case noAgents:
		return nil, nil
	}
	fields := strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	if len(fields) == 0 {
		return nil, invalidChoice(answer, len(detected))
	}
	var chosen []agents.Agent
	picked := make([]bool, len(detected))
	for _, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 1 || n > len(detected) {
			return nil, invalidChoice(field, len(detected))
		}
		if !picked[n-1] {
			picked[n-1] = true
			chosen = append(chosen, detected[n-1])
		}
	}
	return chosen, nil
}

func invalidChoice(answer string, count int) error {
	return fmt.Errorf("invalid choice %q: type numbers from 1 to %d, or %q", answer, count, noAgents)
}

// printManualEntry tells the user how to add the server to an agent that
// setup does not know.
func (s *session) printManualEntry() {
	e := s.deps.Entry
	args := e.Args
	if args == nil {
		args = []string{} // encode as [], not null
	}
	encoded, _ := json.Marshal(args) // a slice of strings always encodes
	s.printf("No supported agents found. Add this server to your agent by hand: command %s, args %s.\n", e.Command, encoded)
}

// addAll adds the server to each chosen agent. A failure is printed and the
// rest still run; the returned error names the agents that failed.
func (s *session) addAll(ctx context.Context, chosen []agents.Agent) error {
	var failed []string
	for _, a := range chosen {
		res, err := a.Add(ctx, s.deps.Entry)
		if err != nil {
			s.printf("✗ %s: %v\n", a.Name(), err)
			failed = append(failed, a.Name())
			continue
		}
		s.printf("%s\n", addedLine(a.Name(), res))
	}
	if len(failed) > 0 {
		return fmt.Errorf("setup finished with errors for: %s", strings.Join(failed, ", "))
	}
	return nil
}

// addedLine reports a successful add: the agent, how to restart it and where
// the backup of its old configuration went.
func addedLine(name string, res agents.Result) string {
	line := "✓ " + name + ": added."
	if res.Restart != "" {
		line += " " + res.Restart
	}
	if res.Backup != "" {
		line += " (backup: " + res.Backup + ")"
	}
	return line
}

// editsLibrary reports whether toolsets include any that write to the library.
func editsLibrary(toolsets []string) bool {
	return slices.ContainsFunc(toolsets, func(t string) bool { return slices.Contains(editingToolsets, t) })
}

// normalizeURL trims spaces and trailing slashes.
func normalizeURL(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

// countLibraries phrases a library count: "1 library", "2 libraries".
func countLibraries(n int) string {
	if n == 1 {
		return "1 library"
	}
	return strconv.Itoa(n) + " libraries"
}

// withoutToken returns err with every occurrence of token in its message
// masked, so a server or proxy that echoes the credentials back cannot put
// the token on the screen. An error that does not mention the token comes
// back unchanged.
func withoutToken(err error, token string) error {
	if token == "" || !strings.Contains(err.Error(), token) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), token, "***"))
}
