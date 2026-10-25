package schemacheck_test

import (
	"os"
	"path"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/config"
)

func TestConfigSchemaParity(t *testing.T) {
	t.Parallel()
	fsys := os.DirFS(schemasDir)
	for _, group := range []string{"valid", "invalid"} {
		for _, name := range examples(t, fsys, path.Join("examples", "config", group)) {
			t.Run(group+"/"+path.Base(name), func(t *testing.T) {
				t.Parallel()
				_, err := config.Parse(read(t, fsys, name))
				if (err == nil) != (group == "valid") {
					t.Fatalf("config parser/schema disagree: %v", err)
				}
			})
		}
	}
}
