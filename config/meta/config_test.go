package meta

import "testing"

func TestConfig_Source(t *testing.T) {
	cases := []struct {
		source string
		schema string
		addr   string
	}{
		{"file://etc/", "file", "etc/"},
		{"etcd://127.0.0.1:2379,127.0.0.1:2380", "etcd", "127.0.0.1:2379,127.0.0.1:2380"},
		{"etcd://", "etcd", ""},
		{"noschema", "", ""}, // used to return garbage "schema"
		{"a", "", ""},        // used to panic
		{"", "", ""},
		{"://empty-schema", "", "empty-schema"},
	}

	for _, c := range cases {
		cfg := Config{Source: c.source}
		if got := cfg.SourceSchema(); got != c.schema {
			t.Errorf("Source(%q).SourceSchema() = %q, want %q", c.source, got, c.schema)
		}
		if got := cfg.SourceAddr(); got != c.addr {
			t.Errorf("Source(%q).SourceAddr() = %q, want %q", c.source, got, c.addr)
		}
	}
}
