package domain

// validDirectorSceneControls keeps new view intent compatible with saved scenes.
func validDirectorSceneControls(c DirectorConfig) bool {
	if !directorChoice(c.Mode, "", "static", "dynamic") {
		return false
	}
	return c.Ground == nil || c.Ground.Color == "" || subtitleColor.MatchString(c.Ground.Color)
}
