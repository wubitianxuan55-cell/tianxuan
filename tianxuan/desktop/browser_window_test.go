package main

import "testing"

func TestOpenBrowserWindowRejectsInvalidURL(t *testing.T) {
	a := &App{}
	for _, u := range []string{"", "   ", "not a url", "file:///c:/x", "ftp://example.com"} {
		if err := a.OpenBrowserWindow(u); err == nil {
			t.Fatalf("OpenBrowserWindow(%q) should fail", u)
		}
	}
}
