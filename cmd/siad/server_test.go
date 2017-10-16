package main

import "testing"

// TestLatestRelease tests that the latestRelease function properly processes a
// set of GitHub releases, returning the release with the highest version
// number.
func TestLatestRelease(t *testing.T) {
	tests := []struct {
		releases    []githubRelease
		expectedTag string
	}{
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "v3.0.7"},
				{TagName: "lts-v2.0.0"},
			},
			expectedTag: "v3.0.7",
		},
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "v3.0.7"},
				{TagName: "v5.2.2"},
			},
			expectedTag: "v5.2.2",
		},
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "lts-v1.0.7"},
				{TagName: "lts-v1.0.5"},
			},
			expectedTag: "", // no non-LTS versions
		},
		{
			releases: []githubRelease{
				{TagName: "v1.0.4"},
				{TagName: "v1.0.7"},
				{TagName: "v1.0.5"},
			},
			expectedTag: "v1.0.7",
		},
		{
			releases: []githubRelease{
				{TagName: "v1.0.4"},
				{TagName: "v1.0.4.1"},
				{TagName: "v1.0.4-patch1"},
			},
			expectedTag: "v1.0.4.1", // -patch is invalid
		},
		{
			releases: []githubRelease{
				{TagName: "abc"},
				{TagName: "def"},
				{TagName: "ghi"},
			},
			expectedTag: "", // invalid version strings
		},
	}
	for i, test := range tests {
		r, _ := latestRelease(test.releases)
		if r.TagName != test.expectedTag {
			t.Errorf("test %v failed: expected %q, got %q", i, test.expectedTag, r.TagName)
		}
	}
}
