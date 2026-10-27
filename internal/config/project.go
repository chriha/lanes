package config

import "errors"

// ProjectTemplate returns a self-documenting project config. forgeType and repo are the forge detected
// from the git origin (see provider.Forge); an empty forgeType means none was detected.
func ProjectTemplate(forgeType, repo string) string {
	return ""
}

// InitProject writes ProjectTemplate(forgeType, repo) to dir/.lanes.toml and returns its path.
// It never overwrites: an existing file is an error wrapping fs.ErrExist.
func InitProject(dir, forgeType, repo string) (string, error) {
	return "", errors.New("not implemented")
}
