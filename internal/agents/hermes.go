package agents

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

// hermesServersKey is the key that holds the MCP servers in Hermes's config.
const hermesServersKey = "mcp_servers"

// YAML tags of the nodes setHermesServer builds or matches.
const (
	yamlMap  = "!!map"
	yamlSeq  = "!!seq"
	yamlStr  = "!!str"
	yamlBool = "!!bool"
	yamlNull = "!!null"
)

func hermes(env Env) Agent {
	home := hermesHome(env)
	return fileAgent{
		about: about{
			id: "hermes", name: "Hermes Agent", restart: "Restart Hermes Agent.",
			bin: "hermes", dir: home, lookPath: env.LookPath,
		},
		path: filepath.Join(home, "config.yaml"),
		edit: setHermesServer,
		now:  env.Now,
	}
}

// hermesHome is $HERMES_HOME, else %LOCALAPPDATA%\hermes on Windows, else
// ~/.hermes.
func hermesHome(env Env) string {
	if dir := env.Getenv("HERMES_HOME"); dir != "" {
		return dir
	}
	if env.GOOS == "windows" {
		return filepath.Join(cmp.Or(env.Getenv("LOCALAPPDATA"), filepath.Join(env.Home, "AppData", "Local")), "hermes")
	}
	return filepath.Join(env.Home, ".hermes")
}

// setHermesServer sets mcp_servers.<ServerName> in the YAML document old to
// {command, args, enabled: true}. It edits the node tree, so comments and the
// rest of the document survive; the output is indented by 2 spaces. A file of
// comments alone decodes to nothing, so there the new mapping is added after
// the comments instead.
func setHermesServer(old []byte, e Entry) ([]byte, error) {
	doc, err := decodeYAMLDocument(old)
	if err != nil {
		return nil, fmt.Errorf("not valid YAML: %w", err)
	}
	commentsOnly := len(doc.Content) == 0 && len(bytes.TrimSpace(old)) > 0
	if len(doc.Content) == 0 || isNull(doc.Content[0]) {
		doc.Content = []*yaml.Node{newMapping()}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("the top level is not a mapping")
	}
	servers, err := childMapping(root, hermesServersKey)
	if err != nil {
		return nil, err
	}
	setChild(servers, ServerName, hermesEntry(e))

	out, err := encodeYAML(doc)
	if err != nil || !commentsOnly {
		return out, err
	}
	sep := []byte{}
	if !bytes.HasSuffix(old, []byte("\n")) {
		sep = []byte("\n")
	}
	return slices.Concat(old, sep, out), nil
}

// decodeYAMLDocument decodes data, which must hold at most one YAML document.
// A document with no content (nothing, or only comments) has no child nodes.
func decodeYAMLDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("the file holds more than one document")
	}
	doc.Kind = yaml.DocumentNode
	return &doc, nil
}

// encodeYAML encodes doc with a 2-space indent.
func encodeYAML(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	return buf.Bytes(), nil
}

// childMapping returns the mapping under key in the mapping m. It adds an
// empty one when key is missing or null.
func childMapping(m *yaml.Node, key string) (*yaml.Node, error) {
	i := valueIndex(m, key)
	switch {
	case i < 0:
		child := newMapping()
		m.Content = append(m.Content, strNode(key), child)
		return child, nil
	case isNull(m.Content[i]):
		m.Content[i] = newMapping()
		return m.Content[i], nil
	case m.Content[i].Kind == yaml.MappingNode:
		return m.Content[i], nil
	default:
		return nil, fmt.Errorf("%q is not a mapping", key)
	}
}

// setChild sets key in the mapping m to value, replacing an existing value.
func setChild(m *yaml.Node, key string, value *yaml.Node) {
	if i := valueIndex(m, key); i >= 0 {
		m.Content[i] = value
		return
	}
	m.Content = append(m.Content, strNode(key), value)
}

// valueIndex returns the index in m.Content of the value stored under key, or
// -1 when the mapping m has no such key.
func valueIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == key {
			return i + 1
		}
	}
	return -1
}

func hermesEntry(e Entry) *yaml.Node {
	args := &yaml.Node{Kind: yaml.SequenceNode, Tag: yamlSeq}
	for _, a := range e.Args {
		args.Content = append(args.Content, strNode(a))
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: yamlMap, Content: []*yaml.Node{
		strNode("command"), strNode(e.Command),
		strNode("args"), args,
		strNode("enabled"), {Kind: yaml.ScalarNode, Tag: yamlBool, Value: "true"},
	}}
}

func newMapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: yamlMap} }

// strNode is a string scalar. The encoder quotes it when it would otherwise
// read back as another type, such as "true" or "1".
func strNode(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStr, Value: s} }

func isNull(n *yaml.Node) bool { return n.Kind == yaml.ScalarNode && n.Tag == yamlNull }
