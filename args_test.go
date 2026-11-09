package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	wd := filepath.Join(string(filepath.Separator)+"music", "in")
	cases := []struct {
		args []string
		want command
		err  bool
	}{
		{nil, command{Kind: "settings"}, false},
		{[]string{"--register"}, command{Kind: "register"}, false},
		{[]string{"--unregister"}, command{Kind: "unregister"}, false},
		{[]string{"convert", "--rule", "to-mp3", "a b.flac"},
			command{Kind: "convert", Rule: "to-mp3", Files: []string{filepath.Join(wd, "a b.flac")}}, false},
		{[]string{"convert", "--rule=to-wav", "--", "--odd.flac", "x.mp3"},
			command{Kind: "convert", Rule: "to-wav", Files: []string{filepath.Join(wd, "--odd.flac"), filepath.Join(wd, "x.mp3")}}, false},
		{[]string{"convert", "a.flac"}, command{}, true},
		{[]string{"convert", "--rule", "to-mp3"}, command{}, true},
		{[]string{"convert", "--bogus", "a"}, command{}, true},
		{[]string{"explode"}, command{}, true},
	}
	for _, c := range cases {
		got, err := parseArgs(c.args, wd)
		if (err != nil) != c.err || (!c.err && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("parseArgs(%q) = %+v, %v; want %+v, err %v", c.args, got, err, c.want, c.err)
		}
	}
}
