package main

import "testing"

func TestConfiguredPort(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
		valid bool
	}{
		{value: "", want: 0, valid: true},
		{value: "8090", want: 8090, valid: true},
		{value: "0"},
		{value: "70000"},
		{value: "news"},
	} {
		got, err := configuredPort(test.value)
		if (err == nil) != test.valid {
			t.Errorf("configuredPort(%q) error = %v", test.value, err)
		} else if err == nil && got != test.want {
			t.Errorf("configuredPort(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}
