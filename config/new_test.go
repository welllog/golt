package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/welllog/golib/testz"
)

func TestFromFile_UppercaseExt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "meta.YAML")
	testz.Nil(t, os.WriteFile(file, []byte("[]\n"), 0644))

	_, err := FromFile(file)
	testz.Nil(t, err)
}
