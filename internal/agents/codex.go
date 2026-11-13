package agents

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// codexHeader starts our table in Codex's config.toml.
const codexHeader = "[mcp_servers." + ServerName + "]"

func codex(env Env) Agent {
	home := cmp.Or(env.Getenv("CODEX_HOME"), filepath.Join(env.Home, ".codex"))
	return fileAgent{
		about: about{
			id: "codex", name: "Codex", restart: "Restart Codex.",
			bin: "codex", dir: home, lookPath: env.LookPath,
		},
		path: filepath.Join(home, "config.toml"),
		edit: setCodexServer,
		now:  env.Now,
	}
}

// setCodexServer replaces our table in the TOML document old, or appends it,
// leaving every other line as it was. The result must parse, so an entry
// written in another form (an inline table, say) gives an error rather than a
// broken file. Our entry in the result must hold exactly the command and args
// we wrote: anything else, such as an env subtable further down the file, is
// left from an old entry, and setup refuses rather than keep it.
func setCodexServer(old []byte, e Entry) ([]byte, error) {
	if _, err := decodeTOML(old); err != nil {
		return nil, fmt.Errorf("not valid TOML: %w", err)
	}
	text := replaceCodexBlock(string(old), codexBlock(e))
	doc, err := decodeTOML([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("the edited file would not be valid TOML (is mcp_servers.%s defined in another form?): %w",
			ServerName, err)
	}
	if !onlyCommandAndArgs(doc) {
		return nil, fmt.Errorf("the existing [mcp_servers.%s…] entry has settings outside its table that setup "+
			"cannot replace; remove that entry by hand and run setup again", ServerName)
	}
	return []byte(text), nil
}

func decodeTOML(data []byte) (map[string]any, error) {
	var doc map[string]any
	err := toml.Unmarshal(data, &doc)
	return doc, err
}

// onlyCommandAndArgs reports whether mcp_servers.<ServerName> in doc is a
// table with exactly the keys command and args.
func onlyCommandAndArgs(doc map[string]any) bool {
	servers, _ := doc["mcp_servers"].(map[string]any)
	entry, ok := servers[ServerName].(map[string]any)
	if !ok || len(entry) != 2 {
		return false
	}
	_, hasCommand := entry["command"]
	_, hasArgs := entry["args"]
	return hasCommand && hasArgs
}

// codexServer is what codexBlock writes under our table's header.
type codexServer struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

// codexBlock is our table: the header, command and args.
func codexBlock(e Entry) string {
	body, _ := toml.Marshal(codexServer{Command: e.Command, Args: argsOrEmpty(e.Args)}) // strings cannot fail to encode
	return codexHeader + "\n" + string(body)
}

// replaceCodexBlock swaps our table in text for block. The table runs from its
// header up to the next table header that isn't one of its own subtables
// ([mcp_servers.audiobookshelf.env], say); blank lines and comments just
// before that next header stay with it. Without the table, block is appended
// after a blank line.
func replaceCodexBlock(text, block string) string {
	lines := strings.SplitAfter(text, "\n")
	start := slices.IndexFunc(lines, isCodexHeader)
	if start < 0 {
		if strings.TrimSpace(text) == "" {
			return block
		}
		return strings.TrimRight(text, "\r\n") + "\n\n" + block
	}
	end := start + 1
	for end < len(lines) && !endsCodexBlock(lines[end]) {
		end++
	}
	for end > start+1 && isBlankOrComment(lines[end-1]) {
		end--
	}
	return strings.Join(lines[:start], "") + block + strings.Join(lines[end:], "")
}

// isCodexHeader reports whether line is our table's header, possibly followed
// by a comment.
func isCodexHeader(line string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), codexHeader)
	rest = strings.TrimSpace(rest)
	return ok && (rest == "" || strings.HasPrefix(rest, "#"))
}

// endsCodexBlock reports whether line is a table header outside our table.
func endsCodexBlock(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "[") && !strings.HasPrefix(t, "[mcp_servers."+ServerName+".")
}

func isBlankOrComment(line string) bool {
	t := strings.TrimSpace(line)
	return t == "" || strings.HasPrefix(t, "#")
}
