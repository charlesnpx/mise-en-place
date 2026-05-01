package artifact

import (
	"fmt"
	"os"
	"path/filepath"
)

// LocalSkillSource returns a local:<path> reference to a skill's tarball
// generated on-the-fly from its source directory. This lets development
// installs work without going through a release pipeline.
//
// In production, ResolveURL (TODO) maps a (skill, version) pair to a
// GitHub Release asset URL.
func LocalSkillSource(skillDir string) (source string, body []byte, err error) {
	body, err = TarGzDir(skillDir)
	if err != nil {
		return "", nil, err
	}
	tmp, err := os.CreateTemp("", "skill-*.tgz")
	if err != nil {
		return "", nil, err
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		return "", nil, err
	}
	abs, err := filepath.Abs(tmp.Name())
	if err != nil {
		return "", nil, err
	}
	return "local:" + abs, body, nil
}

// ResolveURL returns the GitHub Release asset URL for a managed skill version.
// Repo is in "<owner>/<repo>" form.
func ResolveURL(repo, skill, version string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s/%s-%s.tar.gz",
		repo, skill, version, skill, version)
}
