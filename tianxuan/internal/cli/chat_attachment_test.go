package cli

import (
	"reflect"
	"testing"
)

func TestWithAttachmentRefs(t *testing.T) {
	attachments := []chatAttachment{
		{Path: ".tianxuan/attachments/clipboard-20260601-010203.000001.png"},
		{Path: ".tianxuan/attachments/clipboard-20260601-010204.000002.jpg"},
	}
	got := withAttachmentRefs("describe", attachments)
	want := "describe @.tianxuan/attachments/clipboard-20260601-010203.000001.png @.tianxuan/attachments/clipboard-20260601-010204.000002.jpg"
	if got != want {
		t.Fatalf("withAttachmentRefs = %q, want %q", got, want)
	}
}

func TestDisplayLineForImageRefs(t *testing.T) {
	got := displayLineForImageRefs("describe @.tianxuan/attachments/clipboard-20260601-010203.000001.png @.tianxuan/attachments/clipboard-20260601-010204.000002-000002.jpg")
	want := "describe [image1] [image2]"
	if got != want {
		t.Fatalf("displayLineForImageRefs = %q, want %q", got, want)
	}
}

func TestPastedImageSources(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
		ok   bool
	}{
		{
			name: "data URL",
			text: "data:image/png;base64,aaa",
			want: []string{"data:image/png;base64,aaa"},
			ok:   true,
		},
		{
			name: "markdown images",
			text: "![a](/tmp/a.png)\n![b](file:///tmp/b.jpg)",
			want: []string{"/tmp/a.png", "file:///tmp/b.jpg"},
			ok:   true,
		},
		{
			name: "plain text",
			text: "hello /tmp/a.png",
			ok:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := pastedImageSources(c.text)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("sources = %v, want %v", got, c.want)
			}
		})
	}
}
