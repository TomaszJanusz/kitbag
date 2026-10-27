package manifest_test

import (
	"reflect"
	"testing"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{"schema=1", "schema=1\n[skills]\ndirs=['skills']", "schema=2", "schema=3", "schema=2\n[targets.c]\nagent='claude'\n[targets.c.instances.a]\ndir='~/.a'\nhosts=['mac']\n[hosts.mac.targets.c.instances.a]\nenabled=false", "schema=1\n[[files]]\nsrc='a'\ndest='~/a'", "schema=1\n[targets.c]\nagent='claude'", "schema=[", "schema=1\n[hosts.mac]\nmode='copy'"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		first, err := manifest.Parse("fuzz.toml", data)
		second, again := manifest.Parse("fuzz.toml", data)
		if (err == nil) != (again == nil) {
			t.Fatal("nondeterministic acceptance")
		}
		if err != nil {
			if first != nil || second != nil || err.Error() != again.Error() {
				t.Fatal("partial model or nondeterministic diagnostic")
			}
			return
		}
		if first.Schema < manifest.SchemaOldest || first.Schema > manifest.SchemaLatest || !reflect.DeepEqual(first, second) {
			t.Fatal("nondeterministic model")
		}
	})
}
