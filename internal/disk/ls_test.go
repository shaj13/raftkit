package disk

import (
	"os"
	"path/filepath"
	"testing"


	"github.com/stretchr/testify/assert"
)

func TestList(t *testing.T) {
	dir := createTestDir("list", t)
	defer os.RemoveAll(dir)

	table := []struct {
		name        string
		dir         string
		files       []string
		ext         string
		expectedErr bool
	}{
		{
			name:  "it return all files names sorted",
			dir:   dir,
			files: []string{"3.snap", "2.snap", "1.snap"},
			ext:   snapExt,
		},
		{
			name: "it return empty files slice when no files",
			dir:  dir,
			ext:  ".png",
		},
		{
			name:        "it error when dir does not exist",
			dir:         "not exist",
			expectedErr: true,
		},
	}

	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			createTestFiles(tt.dir, tt.files, t)
			got, err := list(tt.dir, tt.ext)
			assert.Equal(t, tt.files, got)
			assert.Equal(t, tt.expectedErr, err != nil)
		})
	}
}

func createTestFiles(dir string, files []string, tb testing.TB) {
	for _, f := range files {
		path := filepath.Join(dir, f)
		f, err := os.Create(path)
		if err != nil {
			tb.Fatal(err)
		}
		f.Close()
	}
}

func createTestDir(name string, tb testing.TB) string {
	dir := filepath.Join(os.TempDir(), name)
	if err := os.Mkdir(dir, 0700); err != nil {
		tb.Fatal(err)
	}
	return dir
}
