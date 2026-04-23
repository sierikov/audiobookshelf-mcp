package audiobookshelf

// AllTools returns every tool the server can offer.
func AllTools(client *ABSClient) []ServerTool {
	var tools []ServerTool
	tools = append(tools, LibraryTools(client)...)
	tools = append(tools, ItemTools(client)...)
	tools = append(tools, PlaybackTools(client)...)
	tools = append(tools, BrowseTools(client)...)
	return tools
}
