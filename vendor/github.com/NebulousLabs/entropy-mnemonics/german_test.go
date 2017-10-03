package mnemonics

import (
	"bytes"
	"crypto/rand"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// TestGermanDictionary checks that the german dictionary is well formed.
func TestGermanDictionary(t *testing.T) {
	// Check for sane constants.
	if German != "german" {
		t.Error("unexpected identifier for german dictionary")
	}
	if GermanUniquePrefixLen != 4 {
		t.Error("unexpected prefix len for german dictionary")
	}

	// Check that the dictionary has well formed elements, and no repeats.
	gerMap := make(map[string]struct{})
	for _, word := range germanDictionary {
		// Check that the word is long enough.
		if utf8.RuneCountInString(word) < GermanUniquePrefixLen {
			t.Fatal("found a short word:", word)
		}

		// Check that the word is normalized.
		newWord := norm.NFC.String(word)
		if newWord != word {
			t.Error("found a non-normalized word:", word)
		}

		// Fetch the prefix, composed of the first GermanUniquePrefixLen runes.
		var prefix []byte
		var runeCount int
		for _, r := range word {
			encR := make([]byte, utf8.RuneLen(r))
			utf8.EncodeRune(encR, r)
			prefix = append(prefix, encR...)

			runeCount++
			if runeCount == GermanUniquePrefixLen {
				break
			}
		}

		// Check that the prefix is unique.
		str := string(prefix)
		_, exists := gerMap[str]
		if exists {
			t.Error("found a prefix conflict:", word)
		}
		gerMap[str] = struct{}{}
	}

	// Do some conversions with the german dictionary.
	for i := 1; i <= 32; i++ {
		for j := 0; j < 5; j++ {
			entropy := make([]byte, i)
			_, err := rand.Read(entropy)
			if err != nil {
				t.Fatal(err)
			}
			phrase, err := ToPhrase(entropy, German)
			if err != nil {
				t.Fatal(err)
			}
			check, err := FromPhrase(phrase, German)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Compare(entropy, check) != 0 {
				t.Error("conversion check failed for the german dictionary")
			}
		}
	}

	// Check that words in a phrase can be altered according to the prefix
	// rule.
	entropy := []byte{1, 2, 3, 4}
	phrase := Phrase{"bete", "Rieglfffffzzzz", "Abundans"}
	check, err := FromPhrase(phrase, German)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(entropy, check) != 0 {
		t.Error("phrase substitution failed")
	}
}
