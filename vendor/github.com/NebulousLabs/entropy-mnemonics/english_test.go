package mnemonics

import (
	"bytes"
	"crypto/rand"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// TestEnglishDictionary checks that the english dictionary is well formed.
func TestEnglishDictionary(t *testing.T) {
	// Check for sane constants.
	if English != "english" {
		t.Error("unexpected identifier for english dictionary")
	}
	if EnglishUniquePrefixLen != 3 {
		t.Error("unexpected prefix len for english dictionary")
	}

	// Check that the dictionary has well formed elements, and no repeats.
	engMap := make(map[string]struct{})
	for _, word := range englishDictionary {
		// Check that the word is long enough.
		if utf8.RuneCountInString(word) < EnglishUniquePrefixLen {
			t.Fatal("found a short word:", word)
		}

		// Check that the word is normalized.
		newWord := norm.NFC.String(word)
		if newWord != word {
			t.Error("found a non-normalized word:", word)
		}

		// Fetch the prefix, composed of the first EnglishUniquePrefixLen
		// runes.
		var prefix []byte
		var runeCount int
		for _, r := range word {
			encR := make([]byte, utf8.RuneLen(r))
			utf8.EncodeRune(encR, r)
			prefix = append(prefix, encR...)

			runeCount++
			if runeCount == EnglishUniquePrefixLen {
				break
			}
		}

		// Check that the prefix is unique.
		str := string(prefix)
		_, exists := engMap[str]
		if exists {
			t.Error("found a prefix conflict:", word)
		}
		engMap[str] = struct{}{}
	}

	// Do some conversions with the english dictionary.
	for i := 1; i <= 32; i++ {
		for j := 0; j < 5; j++ {
			entropy := make([]byte, i)
			_, err := rand.Read(entropy)
			if err != nil {
				t.Fatal(err)
			}
			phrase, err := ToPhrase(entropy, English)
			if err != nil {
				t.Fatal(err)
			}
			check, err := FromPhrase(phrase, English)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Compare(entropy, check) != 0 {
				t.Error("conversion check failed for the english dictionary")
			}
		}
	}

	// Check that words in a phrase can be altered according to the prefix
	// rule.
	entropy := []byte{1, 2, 3, 4}
	phrase := Phrase{"chladsf", "syr", "afiezzz"}
	check, err := FromPhrase(phrase, English)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(entropy, check) != 0 {
		t.Error("phrase substitution failed")
	}
}
