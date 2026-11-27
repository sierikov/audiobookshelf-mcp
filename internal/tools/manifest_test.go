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

	// The made-cover tools are there only when the server has a Gemini key, and
	// the manifest lists every tool a user may get.
	tools := AllTools(Services{CoverArt: NewMockCoverArtService(t)})

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

// geminiKeyEnv is the variable the server reads its Gemini API key from.
const geminiKeyEnv = "AUDIOBOOKSHELF_MCP_GEMINI_API_KEY"

// TestManifestLetsTheUserSetTheGeminiKey keeps an extension install able to
// turn the made-cover tools on: an optional, secret field whose value reaches
// the server through its environment, the way the token's does.
func TestManifestLetsTheUserSetTheGeminiKey(t *testing.T) {
	data, err := os.ReadFile("../../manifest.json")
	require.NoError(t, err)

	var manifest struct {
		Server struct {
			MCPConfig struct {
				Env map[string]string `json:"env"`
			} `json:"mcp_config"`
		} `json:"server"`
		UserConfig map[string]map[string]any `json:"user_config"`
	}
	require.NoError(t, json.Unmarshal(data, &manifest))

	assert.Equal(t, map[string]any{
		"type":        "string",
		"title":       "Gemini API key (optional, paid)",
		"description": "For making new covers with Google's Nano Banana models, about $0.07 to $0.16 a cover. Leave empty to keep made covers off.",
		"sensitive":   true,
		"required":    false,
	}, manifest.UserConfig["gemini_api_key"])
	assert.Equal(t, "${user_config.gemini_api_key}", manifest.Server.MCPConfig.Env[geminiKeyEnv])
	// The token's field is the model: its shape is the one the host knows.
	assert.Equal(t, "${user_config.audiobookshelf_token}", manifest.Server.MCPConfig.Env["AUDIOBOOKSHELF_TOKEN"])
}

// TestServerJSONListsTheGeminiKey keeps a registry install able to set the key.
func TestServerJSONListsTheGeminiKey(t *testing.T) {
	data, err := os.ReadFile("../../server.json")
	require.NoError(t, err)

	var server struct {
		Packages []struct {
			RegistryType         string `json:"registryType"`
			EnvironmentVariables []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				IsRequired  *bool  `json:"isRequired"`
				IsSecret    *bool  `json:"isSecret"`
			} `json:"environmentVariables"`
		} `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(data, &server))

	found := false
	for _, pkg := range server.Packages {
		if pkg.RegistryType != "npm" {
			continue
		}
		for _, v := range pkg.EnvironmentVariables {
			if v.Name != geminiKeyEnv {
				continue
			}
			found = true
			require.NotNil(t, v.IsRequired)
			require.NotNil(t, v.IsSecret)
			assert.False(t, *v.IsRequired)
			assert.True(t, *v.IsSecret)
			assert.Equal(t, "Gemini API key for making new covers (paid, about $0.07 to $0.16 a cover); not needed after running setup", v.Description)
		}
	}
	assert.True(t, found, "server.json lists %s for the npm package", geminiKeyEnv)
}
