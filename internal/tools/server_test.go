package tools

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connect starts srv in memory and returns what a client sees when it
// initializes.
func connect(t *testing.T, srv *mcp.Server) *mcp.InitializeResult {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	_, err := srv.Connect(t.Context(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs.InitializeResult()
}

func TestNewServer_ReportsVersionAndInstructions(t *testing.T) {
	init := connect(t, NewServer(Services{}, Options{Version: "1.2.3", Instructions: "say hi"}))

	assert.Equal(t, "1.2.3", init.ServerInfo.Version)
	assert.Equal(t, baseInstructions+"\n\nsay hi", init.Instructions)
}

func TestNewServer_SendsTheBaseInstructionsAlone(t *testing.T) {
	init := connect(t, NewServer(Services{}, Options{Version: "1.2.3"}))

	assert.Equal(t, baseInstructions, init.Instructions)
}
