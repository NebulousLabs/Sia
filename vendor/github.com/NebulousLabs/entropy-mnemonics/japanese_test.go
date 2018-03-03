package mnemonics

import (
	"bytes"
	"crypto/rand"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// TestJapaneseDictionary checks that the japanese dictionary is
// well formed.
func TestJapanesesDictionary(t *testing.T) {
	// Check for sane constants.
	if Japanese != "japanese" {
		t.Error("unexpected identifier for japanese dictionary")
	}
	if JapaneseUniquePrefixLen != 3 {
		t.Error("unexpected prefix len for japanese dictionary")
	}

	// Check that the dictionary has well formed elements, and no repeats.
	japMap := make(map[string]struct{})
	for _, word := range japaneseDictionary {
		// Check that the word is long enough.
		if utf8.RuneCountInString(word) < JapaneseUniquePrefixLen {
			t.Fatal("found a short word:", word)
		}

		// Check that the word is normalized.
		newWord := norm.NFC.String(word)
		if newWord != word {
			t.Error("found a non-normalized word:", word)
		}

		// Fetch the prefix, composed of the first JapaneseUniquePrefixLen
		// runes.
		var prefix []byte
		var runeCount int
		for _, r := range word {
			encR := make([]byte, utf8.RuneLen(r))
			utf8.EncodeRune(encR, r)
			prefix = append(prefix, encR...)

			runeCount++
			if runeCount == JapaneseUniquePrefixLen {
				break
			}
		}

		// Check that the prefix is unique.
		str := string(prefix)
		_, exists := japMap[str]
		if exists {
			t.Error("found a prefix conflict:", word)
		}
		japMap[str] = struct{}{}
	}

	// Do some conversions with the japanese dictionary.
	for i := 1; i <= 32; i++ {
		for j := 0; j < 5; j++ {
			entropy := make([]byte, i)
			_, err := rand.Read(entropy)
			if err != nil {
				t.Fatal(err)
			}
			phrase, err := ToPhrase(entropy, Japanese)
			if err != nil {
				t.Fatal(err)
			}
			check, err := FromPhrase(phrase, Japanese)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Compare(entropy, check) != 0 {
				t.Error("conversion check failed for the japanese dictionary")
			}
		}
	}

	// Check that words in a phrase can be altered according to the prefix
	// rule.
	entropy := []byte{1, 2, 3, 4}
	phrase := Phrase{"えんち", "としょbar", "あふれbaz"}
	check, err := FromPhrase(phrase, Japanese)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(entropy, check) != 0 {
		t.Error("phrase substitution failed")
	}
}
