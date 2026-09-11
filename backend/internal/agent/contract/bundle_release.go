package contract

import (
	_ "embed"
	"errors"
)

//go:embed manifests/build-storygraph.json
var pinnedStoryGraphBundleManifest []byte

func PinnedStoryGraphBundleManifest() (BundleContentManifest, error) {
	manifest, _, err := DecodeStoryGraphBundleManifest(pinnedStoryGraphBundleManifest)
	if err != nil || manifest.ContentHash != StoryGraphSkillBundleHash {
		return BundleContentManifest{}, errors.New("invalid pinned StoryGraph bundle manifest")
	}
	return manifest, nil
}
