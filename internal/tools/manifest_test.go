package tools

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type manifestTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// TestManifestToolsMatchServer keeps the tool list in manifest.json, which
// registries and marketplaces display, in sync with the tools the server registers.
func TestManifestToolsMatchServer(t *testing.T) {
	data, err := os.ReadFile("../../manifest.json")
	require.NoError(t, err)

	var manifest struct {
		Tools []manifestTool `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(data, &manifest))

	declared := make(map[string]string, len(manifest.Tools))
	for _, mt := range manifest.Tools {
		declared[mt.Name] = mt.Description
	}

	tools := AllTools(Services{})

	serverNames := make([]string, 0, len(tools))
	for _, st := range tools {
		serverNames = append(serverNames, st.Tool.Name)
	}
	manifestNames := make([]string, 0, len(manifest.Tools))
	for _, mt := range manifest.Tools {
		manifestNames = append(manifestNames, mt.Name)
	}
	require.ElementsMatch(t, serverNames, manifestNames, "manifest.json tools differ from registered tools")

	for _, st := range tools {
		assert.True(t, strings.HasPrefix(declared[st.Tool.Name], st.Tool.Description),
			"manifest description for %s should start with %q, got %q", st.Tool.Name, st.Tool.Description, declared[st.Tool.Name])
	}
}
