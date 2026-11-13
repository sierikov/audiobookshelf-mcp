package agents

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

// jsonServersKey is the key that holds the MCP servers in every JSON config.
const jsonServersKey = "mcpServers"

// claudeDesktopMSIX is the folder of the Microsoft Store (MSIX) build of
// Claude Desktop under %LOCALAPPDATA%\Packages.
const claudeDesktopMSIX = "Claude_pzs8sxrjxfjjc"

// jsonFileAgent returns an agent configured through a JSON file holding
// {"mcpServers": {"<name>": <entry>}}. entryJSON builds our entry.
func jsonFileAgent(env Env, a about, path string, entryJSON func(Entry) map[string]any) fileAgent {
	a.lookPath = env.LookPath
	return fileAgent{
		about: a,
		path:  path,
		edit: func(old []byte, e Entry) ([]byte, error) {
			return setJSONServer(old, entryJSON(e))
		},
		now: env.Now,
	}
}

func claudeDesktop(env Env) Agent {
	dir := claudeDesktopDir(env)
	return jsonFileAgent(env, about{
		id: "claude-desktop", name: "Claude Desktop", restart: "Quit and reopen Claude Desktop.",
		dir: dir,
	}, filepath.Join(dir, "claude_desktop_config.json"), func(e Entry) map[string]any {
		return commandEntry(npxThroughCmd(env.GOOS, e))
	})
}

// claudeDesktopDir is the folder that holds claude_desktop_config.json.
func claudeDesktopDir(env Env) string {
	switch env.GOOS {
	case "darwin":
		return filepath.Join(env.Home, "Library", "Application Support", "Claude")
	case "windows":
		local := cmp.Or(env.Getenv("LOCALAPPDATA"), filepath.Join(env.Home, "AppData", "Local"))
		msix := filepath.Join(local, "Packages", claudeDesktopMSIX, "LocalCache", "Roaming", "Claude")
		if dirExists(msix) {
			return msix
		}
		roaming := cmp.Or(env.Getenv("APPDATA"), filepath.Join(env.Home, "AppData", "Roaming"))
		return filepath.Join(roaming, "Claude")
	default:
		return filepath.Join(cmp.Or(env.Getenv("XDG_CONFIG_HOME"), filepath.Join(env.Home, ".config")), "Claude")
	}
}

func antigravity(env Env) Agent {
	dir := filepath.Join(env.Home, ".gemini", "config")
	return jsonFileAgent(env, about{
		id: "antigravity", name: "Antigravity CLI", restart: "Restart Antigravity CLI.",
		// Not the agy binary: that is also the Antigravity IDE's launcher.
		dir: dir,
	}, filepath.Join(dir, "mcp_config.json"), commandEntry)
}

func gemini(env Env) Agent {
	return jsonFileAgent(env, about{
		id: "gemini", name: "Gemini CLI", restart: "Restart Gemini CLI.",
		bin: "gemini",
	}, filepath.Join(env.Home, ".gemini", "settings.json"), commandEntry)
}

func cursor(env Env) Agent {
	dir := filepath.Join(env.Home, ".cursor")
	return jsonFileAgent(env, about{
		id: "cursor", name: "Cursor", restart: "Restart Cursor.",
		dir: dir,
	}, filepath.Join(dir, "mcp.json"), func(e Entry) map[string]any {
		m := commandEntry(e)
		m["type"] = "stdio"
		return m
	})
}

// commandEntry is the common JSON entry: {"command": …, "args": […]}.
func commandEntry(e Entry) map[string]any {
	return map[string]any{"command": e.Command, "args": argsOrEmpty(e.Args)}
}

// utf8BOM is the byte order mark some Windows editors put at the start of a
// UTF-8 file.
const utf8BOM = "\xEF\xBB\xBF"

// setJSONServer sets mcpServers.<ServerName> in the JSON object old to entry
// and keeps everything else. Empty input counts as an empty object. A leading
// byte order mark is dropped, and the result is written without one. Numbers
// are kept as written; key order is not.
func setJSONServer(old []byte, entry map[string]any) ([]byte, error) {
	root, err := decodeJSONObject(bytes.TrimPrefix(old, []byte(utf8BOM)))
	if err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	servers := map[string]any{}
	switch v := root[jsonServersKey].(type) {
	case nil:
	case map[string]any:
		servers = v
	default:
		return nil, fmt.Errorf("%q is not an object", jsonServersKey)
	}
	servers[ServerName] = entry
	root[jsonServersKey] = servers

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encoding JSON: %w", err)
	}
	return buf.Bytes(), nil
}

// decodeJSONObject decodes data, which must hold a single JSON object (or
// nothing, or null), keeping numbers as json.Number.
func decodeJSONObject(data []byte) (map[string]any, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(data)) == 0 {
		return root, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the top-level object")
	}
	if root == nil { // the file holds null
		root = map[string]any{}
	}
	return root, nil
}
