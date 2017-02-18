package contractor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

func tempFile(t interface {
	Fatal(...interface{})
}, name string) (*os.File, func()) {
	f, err := os.Create(filepath.Join(build.TempDir("contractor", name)))
	if err != nil {
		t.Fatal(err)
	}
	return f, func() {
		f.Close()
		os.RemoveAll(f.Name())
	}
}

func tempJournal(t interface {
	Fatal(...interface{})
}, obj interface{}, name string) (*journal, func()) {
	j, err := openJournal(filepath.Join(build.TempDir("contractor", name)), obj)
	if err != nil {
		t.Fatal(err)
	}
	return j, func() {
		j.Close()
		os.RemoveAll(j.filename)
	}
}

func TestJournal(t *testing.T) {
	type bar struct {
		Z int `json:"z"`
	}
	type foo struct {
		X int   `json:"x"`
		Y []bar `json:"y"`
	}

	j, cleanup := tempJournal(t, foo{Y: []bar{}}, "TestJournal")
	defer cleanup()

	us := []journalUpdate{
		newJournalUpdate("x", 7),
		newJournalUpdate("y.0", bar{}),
		newJournalUpdate("y.0.z", 3),
	}
	if err := j.update(us); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	var f foo
	j2, err := openJournal(j.filename, &f)
	if err != nil {
		t.Fatal(err)
	}
	j2.Close()
	if f.X != 7 || len(f.Y) != 1 || f.Y[0].Z != 3 {
		t.Fatal("openJournal applied updates incorrectly:", f)
	}
}

func TestJournalCheckpoint(t *testing.T) {
	type bar struct {
		Z int `json:"z"`
	}
	type foo struct {
		X int   `json:"x"`
		Y []bar `json:"y"`
	}

	j, cleanup := tempJournal(t, foo{Y: []bar{}}, "TestJournalCheckpoint")
	defer cleanup()

	if err := j.checkpoint(bar{3}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	var b bar
	j2, err := openJournal(j.filename, &b)
	if err != nil {
		t.Fatal(err)
	}
	j2.Close()
	if b.Z != 3 {
		t.Fatal("checkpoint failed:", b.Z)
	}
}

func TestJournalMalformed(t *testing.T) {
	f, cleanup := tempFile(t, "TestJournalMalformed")
	defer cleanup()

	// write a partially-malformed log
	f.WriteString(`{"foo": 3}
[{"p": "foo", "v": 4}]
[{"p": "foo", "v": 5}`)
	f.Close()

	// load log into foo
	var foo struct {
		Foo int `json:"foo"`
	}
	j, err := openJournal(f.Name(), &foo)
	if err != nil {
		t.Fatal(err)
	}
	j.Close()

	// the last update set should have been discarded
	if foo.Foo != 4 {
		t.Fatal("log was not applied correctly:", foo.Foo)
	}
}

func TestRewritePath(t *testing.T) {
	tests := []struct {
		json string
		path string
		val  string
		exp  string
	}{
		{``, ``, ``, ``},
		{`"foo"`, ``, ``, ``},
		{``, `foo`, ``, ``},
		{``, ``, `"foo"`, `"foo"`},
		{`"foo"`, ``, `"bar"`, `"bar"`},
		// object
		{`{"foo":"bar"}`, `bar`, `"baz"`, `{"foo":"bar"}`},
		{`{"foo":"bar"}`, `foo`, `"baz"`, `{"foo":"baz"}`},
		{`{"foo":"bar", "bar":"baz"}`, `bar`, `"quux"`, `{"foo":"bar", "bar":"quux"}`},
		{`{"foo": {"bar": "baz"}}`, `foo.bar`, `"quux"`, `{"foo": {"bar": "quux"}}`},
		// array
		{`[]`, `foo`, `"bar"`, `[]`},
		{`[1]`, `0`, `"bar"`, `["bar"]`},
		{`[1, 2]`, `0`, `"bar"`, `["bar", 2]`},
		{`[1, 2]`, `1`, `"bar"`, `[1, "bar"]`},
		{`[]`, `0`, `"bar"`, `["bar"]`},
		{`[1]`, `1`, `"bar"`, `[1,"bar"]`},
		{`["foo", "bar"]`, `2`, `"baz"`, `["foo", "bar","baz"]`},
		{`[[1,2], [3,4]]`, `1.0`, `"baz"`, `[[1,2], ["baz",4]]`},
		{`[[1,2], [3,4]]`, `1.2`, `"baz"`, `[[1,2], [3,4,"baz"]]`},
		{`[[1,2], [3,4]]`, `2.0`, `"baz"`, `[[1,2], [3,4]]`},
		{`[[1,2], [3,4]]`, `2`, `"baz"`, `[[1,2], [3,4],"baz"]`},
		// array in object
		{`{"foo": [1,2]}`, `foo.0`, `"bar"`, `{"foo": ["bar",2]}`},
		{`{"foo": [1,2]}`, `foo.2`, `"bar"`, `{"foo": [1,2,"bar"]}`},
		{`{"foo": [1,2]}`, `bar.2`, `"bar"`, `{"foo": [1,2]}`},
		// object in array
		{`[{"foo": "bar"}]`, `0.foo`, `"baz"`, `[{"foo": "baz"}]`},
		{`[{}, {"foo": "bar"}]`, `1.foo`, `"baz"`, `[{}, {"foo": "baz"}]`},
		{`[{"foo": "bar"}]`, `1.foo`, `"baz"`, `[{"foo": "bar"}]`},
		// monster
		{`{"foo": [{}, {"bar": [{"baz":""}]}}]`, `foo.1.bar.0.baz`, `"quux"`, `{"foo": [{}, {"bar": [{"baz":"quux"}]}}]`},
	}
	for _, test := range tests {
		if res := rewritePath([]byte(test.json), test.path, []byte(test.val)); string(res) != test.exp {
			t.Errorf("rewritePath('%s', %q, '%s'): expected '%s', got '%s'", test.json, test.path, test.val, test.exp, res)
		}
	}
}

func TestLocateAccessor(t *testing.T) {
	tests := []struct {
		json string
		acc  string
		loc  int
	}{
		// object
		{`{}`, `foo`, -1},
		{`{"foo":0}`, `foo`, 7},
		{`{"foo":0}`, `bar`, -1},
		{`{"foo":0}3`, `foo`, 7},
		{`{"foo":0} 3`, `foo`, 7},
		{`{"foo":0,"bar":7}`, `bar`, len(`{"foo":0,"bar":`)},
		{`{"foo":0 , "bar":7}`, `bar`, len(`{"foo":0 , "bar":`)},
		{`{"foo":0,"bar":7}3`, `bar`, len(`{"foo":0,"bar":`)},
		{`{"foo":0,"bar":7} 3`, `bar`, len(`{"foo":0,"bar":`)},
		// array
		{`[1,2,3]`, `0`, 1},
		{`[1,2,3]`, `1`, 3},
		{`[1,2,3]`, `2`, 5},
		{`[1,2,3]`, `3`, 6}, // special case
		{`[1,2,3]`, `4`, -1},
		{`[1,2,3]`, `foo`, -1},
		{`[]`, `0`, 1},
		{`[]`, `1`, -1},
		// string
		{`"foo"`, `foo`, -1},
		{`"{\"foo\": 3}"`, `foo`, -1},
		// number
		{`3`, `foo`, -1},
		{`3`, `3`, -1},
	}
	for _, test := range tests {
		if loc := locateAccessor([]byte(test.json), test.acc); loc != test.loc {
			t.Errorf("locateAccessor('%s', %q): expected %v, got %v", test.json, test.acc, test.loc, loc)
		}
	}
}

func TestParseString(t *testing.T) {
	tests := []struct {
		json string
		str  string
		rest string
	}{
		{`""`, ``, ``},
		{`"foo"`, `foo`, ``},
		{`"foo":"bar"`, `foo`, `:"bar"`},
		{`"foo" : "bar"`, `foo`, ` : "bar"`},
		{`"foo\"bar"`, `foo\"bar`, ``},
		{`"foo\"bar":"baz"`, `foo\"bar`, `:"baz"`},
		{`"foo\\\"bar":"baz"`, `foo\\\"bar`, `:"baz"`},
	}
	for _, test := range tests {
		if str, rest := parseString([]byte(test.json)); string(str) != test.str || string(rest) != test.rest {
			t.Errorf("parseString('%s'): expected (%q, '%s'), got (%q, '%s')", test.json, test.str, test.rest, str, rest)
		}
	}
}

func TestConsumeWhitespace(t *testing.T) {
	tests := []struct {
		json string
		rest string
	}{
		{" ", ``},
		{" 3", `3`},
		{"\t", ``},
		{"\t3", `3`},
		{"\n", ``},
		{"\n3", `3`},
		{"\r", ``},
		{"\r3", `3`},
		{" \t\n\r", ``},
		{" \t\n\r3", `3`},
	}
	for _, test := range tests {
		if rest := consumeWhitespace([]byte(test.json)); string(rest) != test.rest {
			t.Errorf("consumeWhitespace('%s'): expected '%s', got '%s'", test.json, test.rest, rest)
		}
	}
}

func TestConsumeSeparator(t *testing.T) {
	tests := []struct {
		json string
		rest string
	}{
		{"[", ``},
		{"[ \r\n\t", ``},
		{"[3", `3`},
		{"[ \r\n\t3", `3`},
		{"{", ``},
		{"{ \r\n\t", ``},
		{"{3", `3`},
		{"{ \r\n\t3", `3`},
		{"}", ``},
		{"} \r\n\t", ``},
		{"}3", `3`},
		{"} \r\n\t3", `3`},
		{"]", ``},
		{"] \r\n\t", ``},
		{"]3", `3`},
		{"] \r\n\t3", `3`},
		{":", ``},
		{": \r\n\t", ``},
		{":3", `3`},
		{": \r\n\t3", `3`},
		{",", ``},
		{", \r\n\t", ``},
		{",3", `3`},
		{", \r\n\t3", `3`},
	}
	for _, test := range tests {
		if rest := consumeSeparator([]byte(test.json)); string(rest) != test.rest {
			t.Errorf("consumeSeparator('%s'): expected '%s', got '%s'", test.json, test.rest, rest)
		}
	}
}

func TestConsumeValue(t *testing.T) {
	tests := []struct {
		json string
		rest string
	}{
		// object
		{`{}`, ``},
		{`{}3`, `3`},
		{`{} 3`, ` 3`},
		{`{"foo":0}`, ``},
		{`{"foo":0}3`, `3`},
		{`{"foo":0} 3`, ` 3`},
		{`{"foo":0,"bar":7}`, ``},
		{`{"foo":0,"bar":7}3`, `3`},
		{`{"foo":0,"bar":7} 3`, ` 3`},
		{`{"foo":0 , "bar":7}`, ``},
		{`{"foo":0 , "bar":7}3`, `3`},
		{`{"foo":0 , "bar":7} 3`, ` 3`},
		{`{"":""}`, ``},
		{`{"":""}3`, `3`},
		{`{"":""} 3`, ` 3`},
		{`{"}":"}"}`, ``},
		{`{"}":"}"}3`, `3`},
		{`{"}":"}"} 3`, ` 3`},
		{`{"":{}}`, ``},
		{`{"":{}}3`, `3`},
		{`{"":{}} 3`, ` 3`},
		{`{"}":["}{"]}`, ``},
		{`{"}":["}{"]}3`, `3`},
		{`{"}":["}{"]} 3`, ` 3`},
		{`{"":{"":{"":{}}}}`, ``},
		{`{"":{"":{"":{}}}}3`, `3`},
		{`{"":{"":{"":{}}}} 3`, ` 3`},
		// array
		{`[]`, ``},
		{`[]3`, `3`},
		{`[] 3`, ` 3`},
		{`[0]`, ``},
		{`[0]3`, `3`},
		{`[0] 3`, ` 3`},
		{`[0,1]`, ``},
		{`[0,1]3`, `3`},
		{`[0,1] 3`, ` 3`},
		{`[0 , 1]`, ``},
		{`[0 , 1]3`, `3`},
		{`[0 , 1] 3`, ` 3`},
		{`["", ""]`, ``},
		{`["", ""]3`, `3`},
		{`["", ""] 3`, ` 3`},
		{`[[], []]`, ``},
		{`[[], []]3`, `3`},
		{`[[], []] 3`, ` 3`},
		{`["[", "]"]`, ``},
		{`["[", "]"]3`, `3`},
		{`["[", "]"] 3`, ` 3`},
		{`[["]", "]"], [{"foo":[]}]]`, ``},
		{`[["]", "]"], [{"foo":[]}]]3`, `3`},
		{`[["]", "]"], [{"foo":[]}]] 3`, ` 3`},
		// string
		{`""`, ``},
		{`"foo"`, ``},
		{`"foo":"bar"`, `:"bar"`},
		{`"foo" : "bar"`, ` : "bar"`},
		{`"foo\"bar"`, ``},
		{`"foo\"bar":"baz"`, `:"baz"`},
		// true, false, null
		{`true`, ``},
		{`false`, ``},
		{`null`, ``},
		{`true3`, `3`},
		{`false3`, `3`},
		{`null3`, `3`},
		{`true 3`, ` 3`},
		{`false 3`, ` 3`},
		{`null 3`, ` 3`},
		// number
		{`-0`, ``},
		{`-0 true`, ` true`},
		{`0`, ``},
		{`0 true`, ` true`},
		{`0.0`, ``},
		{`0.0 true`, ` true`},
		{`1.0`, ``},
		{`1.0 true`, ` true`},
		{`10`, ``},
		{`10 true`, ` true`},
		{`10.1`, ``},
		{`10.1 true`, ` true`},
		{`1e7`, ``},
		{`1e7 true`, ` true`},
		{`1e+7`, ``},
		{`1e+7 true`, ` true`},
		{`1e-7`, ``},
		{`1e-7 true`, ` true`},
		{`1.0e7`, ``},
		{`1.0e7 true`, ` true`},
		{`1.0e+7`, ``},
		{`1.0e+7 true`, ` true`},
		{`1.0e-7`, ``},
		{`1.0e-7 true`, ` true`},
		{`10.1e7`, ``},
		{`10.1e7 true`, ` true`},
		{`10.1e+7`, ``},
		{`10.1e+7 true`, ` true`},
		{`10.1e-7`, ``},
		{`10.1e-7 true`, ` true`},
	}
	for _, test := range tests {
		if rest := consumeValue([]byte(test.json)); string(rest) != test.rest {
			t.Errorf("consumeValue('%s'): expected '%s', got '%s'", test.json, test.rest, rest)
		}
	}
}

func TestConsumeObject(t *testing.T) {
	tests := []struct {
		json string
		rest string
	}{
		{`{}`, ``},
		{`{}3`, `3`},
		{`{} 3`, ` 3`},
		{`{"foo":0}`, ``},
		{`{"foo":0}3`, `3`},
		{`{"foo":0} 3`, ` 3`},
		{`{"foo":0,"bar":7}`, ``},
		{`{"foo":0,"bar":7}3`, `3`},
		{`{"foo":0,"bar":7} 3`, ` 3`},
		{`{"foo":0 , "bar":7}`, ``},
		{`{"foo":0 , "bar":7}3`, `3`},
		{`{"foo":0 , "bar":7} 3`, ` 3`},
		{`{"":""}`, ``},
		{`{"":""}3`, `3`},
		{`{"":""} 3`, ` 3`},
		{`{"}":"}"}`, ``},
		{`{"}":"}"}3`, `3`},
		{`{"}":"}"} 3`, ` 3`},
		{`{"":{}}`, ``},
		{`{"":{}}3`, `3`},
		{`{"":{}} 3`, ` 3`},
		{`{"}":["}{"]}`, ``},
		{`{"}":["}{"]}3`, `3`},
		{`{"}":["}{"]} 3`, ` 3`},
		{`{"":{"":{"":{}}}}`, ``},
		{`{"":{"":{"":{}}}}3`, `3`},
		{`{"":{"":{"":{}}}} 3`, ` 3`},
	}
	for _, test := range tests {
		if rest := consumeObject([]byte(test.json)); string(rest) != test.rest {
			t.Errorf("consumeObject('%s'): expected '%s', got '%s'", test.json, test.rest, rest)
		}
	}
}

func TestConsumeArray(t *testing.T) {
	tests := []struct {
		json string
		rest string
	}{
		{`[]`, ``},
		{`[]3`, `3`},
		{`[] 3`, ` 3`},
		{`[0]`, ``},
		{`[0]3`, `3`},
		{`[0] 3`, ` 3`},
		{`[0,1]`, ``},
		{`[0,1]3`, `3`},
		{`[0,1] 3`, ` 3`},
		{`[0 , 1]`, ``},
		{`[0 , 1]3`, `3`},
		{`[0 , 1] 3`, ` 3`},
		{`["", ""]`, ``},
		{`["", ""]3`, `3`},
		{`["", ""] 3`, ` 3`},
		{`[[], []]`, ``},
		{`[[], []]3`, `3`},
		{`[[], []] 3`, ` 3`},
		{`["[", "]"]`, ``},
		{`["[", "]"]3`, `3`},
		{`["[", "]"] 3`, ` 3`},
		{`[["]", "]"], [{"foo":[]}]]`, ``},
		{`[["]", "]"], [{"foo":[]}]]3`, `3`},
		{`[["]", "]"], [{"foo":[]}]] 3`, ` 3`},
	}
	for _, test := range tests {
		if rest := consumeArray([]byte(test.json)); string(rest) != test.rest {
			t.Errorf("consumeArray('%s'): expected '%s', got '%s'", test.json, test.rest, rest)
		}
	}
}

func TestConsumeString(t *testing.T) {
	tests := []struct {
		json string
		rest string
	}{
		{`""`, ``},
		{`"foo"`, ``},
		{`"foo":"bar"`, `:"bar"`},
		{`"foo" : "bar"`, ` : "bar"`},
		{`"foo\"bar"`, ``},
		{`"foo\"bar":"baz"`, `:"baz"`},
	}
	for _, test := range tests {
		if rest := consumeString([]byte(test.json)); string(rest) != test.rest {
			t.Errorf("consumeString('%s'): expected '%s', got '%s'", test.json, test.rest, rest)
		}
	}
}

func TestConsumeNumber(t *testing.T) {
	tests := []struct {
		json string
		rest string
	}{
		{`-0`, ``},
		{`-0 true`, ` true`},
		{`0`, ``},
		{`0 true`, ` true`},
		{`0.0`, ``},
		{`0.0 true`, ` true`},
		{`1.0`, ``},
		{`1.0 true`, ` true`},
		{`10`, ``},
		{`10 true`, ` true`},
		{`10.1`, ``},
		{`10.1 true`, ` true`},
		{`1e7`, ``},
		{`1e7 true`, ` true`},
		{`1e+7`, ``},
		{`1e+7 true`, ` true`},
		{`1e-7`, ``},
		{`1e-7 true`, ` true`},
		{`1.0e7`, ``},
		{`1.0e7 true`, ` true`},
		{`1.0e+7`, ``},
		{`1.0e+7 true`, ` true`},
		{`1.0e-7`, ``},
		{`1.0e-7 true`, ` true`},
		{`10.1e7`, ``},
		{`10.1e7 true`, ` true`},
		{`10.1e+7`, ``},
		{`10.1e+7 true`, ` true`},
		{`10.1e-7`, ``},
		{`10.1e-7 true`, ` true`},
	}
	for _, test := range tests {
		if rest := consumeNumber([]byte(test.json)); string(rest) != test.rest {
			t.Errorf("consumeNumber('%s'): expected '%s', got '%s'", test.json, test.rest, rest)
		}
	}
}

func BenchmarkUpdateJournal(b *testing.B) {
	f, cleanup := tempFile(b, "BenchmarkUpdateJournal")
	defer cleanup()

	j := &journal{f: f}
	us := []journalUpdate{
		newJournalUpdate("foo.bar", struct{ X, Y int }{3, 4}),
		newJournalUpdate("foo.bar", nil),
	}

	for i := 0; i < b.N; i++ {
		if err := j.update(us); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkApply(b *testing.B) {
	u := newJournalUpdate("foo.bar.baz", "")
	json := []byte(`{"foo": {"bar": {"baz": "quux"}}}`)
	for i := 0; i < b.N; i++ {
		u.apply(json)
	}
}
