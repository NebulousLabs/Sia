package main

import "testing"

// TestLatestLTS tests that the latestLTS function properly processes a set of
// GitHub releases, returning the LTS release with the highest version number.
func TestLatestLTS(t *testing.T) {
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
			expectedTag: "lts-v2.0.0",
		},
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "v3.0.7"},
				{TagName: "lts-v1.1.0"},
			},
			expectedTag: "lts-v1.1.0",
		},
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "v1.0.7"},
				{TagName: "lts-v1.0.5"},
			},
			expectedTag: "lts-v1.0.5",
		},
		{
			releases: []githubRelease{
				{TagName: "v1.0.4"},
				{TagName: "v1.0.7"},
				{TagName: "v1.0.5"},
			},
			expectedTag: "", // no LTS versions
		},
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "lts-v1.0.4.1"},
				{TagName: "lts-v1.0.4-patch1"},
			},
			expectedTag: "lts-v1.0.4.1", // -patch is invalid
		},
		{
			releases: []githubRelease{
				{TagName: "lts-abc"},
				{TagName: "lts-def"},
				{TagName: "lts-ghi"},
			},
			expectedTag: "", // invalid version strings
		},
	}
	for i, test := range tests {
		r, _ := latestLTS(test.releases)
		if r.TagName != test.expectedTag {
			t.Errorf("test %v failed: expected %q, got %q", i, test.expectedTag, r.TagName)
		}
	}
}
