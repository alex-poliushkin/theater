package sdk

import pluginmanifest "github.com/alex-poliushkin/theater/plugin/manifest"

// FinalizeManifest returns a validated manifest with the canonical descriptor
// digest set from the protocol and capability descriptors.
func FinalizeManifest(file pluginmanifest.File) (pluginmanifest.File, error) {
	return pluginmanifest.Finalize(file)
}
