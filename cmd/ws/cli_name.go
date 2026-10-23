package ws

const cliNameWS = "ws"

// CLIName returns the command name.
func CLIName() string {
	return cliNameWS
}

// BinaryAssetName returns the release asset prefix for self-update.
func BinaryAssetName() string {
	return cliNameWS
}
