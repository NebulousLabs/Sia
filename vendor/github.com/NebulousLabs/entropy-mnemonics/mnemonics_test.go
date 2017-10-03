package mnemonics

import (
	"bytes"
	"testing"
)

// TestConversions checks ToPhrase and FromPhrase for consistency and sanity.
func TestConversions(t *testing.T) {
	// Try for value {0}.
	initial := []byte{0}
	phrase, err := ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[0] {
		t.Error("unexpected ToPhrase result")
	}
	final, err := FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {0}")
	}

	// Try for value {1}.
	initial = []byte{1}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[1] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {1}")
	}

	// Try for value {255}.
	initial = []byte{255}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[255] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {255}")
	}

	// Try for value {0, 0}.
	initial = []byte{0, 0}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[256] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {0, 0}")
	}

	// Try for value {1, 0}.
	initial = []byte{1, 0}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[257] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {1, 0}")
	}

	// Try for value {0, 1}.
	initial = []byte{0, 1}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[512] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {0, 1}")
	}

	// Try for value {1, 1}.
	initial = []byte{1, 1}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[513] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {1, 1}")
	}

	// Try for value {2, 1}.
	initial = []byte{2, 1}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[514] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {2, 1}")
	}

	// Try for value {2, 2}.
	initial = []byte{2, 2}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 1 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[770] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {2, 2}")
	}

	// Try for value {abbey, abbey}.
	initial = []byte{90, 5}
	phrase, err = ToPhrase(initial, English)
	if err != nil {
		t.Error(err)
	}
	if len(phrase) != 2 {
		t.Fatal("unexpected phrase length")
	}
	if phrase[0] != englishDictionary[0] {
		t.Error("unexpected ToPhrase result")
	}
	if phrase[1] != englishDictionary[0] {
		t.Error("unexpected ToPhrase result")
	}
	final, err = FromPhrase(phrase, English)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(initial, final) != 0 {
		t.Error("failure for value {abbey, abbey}")
	}

	// Check that all values going from []byte to phrase and back result in the
	// original value, as deep as reasonable.
	for i := 0; i < 256; i++ {
		initial := []byte{byte(i)}
		phrase, err := ToPhrase(initial, English)
		if err != nil {
			t.Fatal(err)
		}
		final, err := FromPhrase(phrase, English)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(initial, final) != 0 {
			t.Error("comparison failed during circular byte check")
		}
	}
	for i := 0; i < 256; i++ {
		for j := 0; j < 256; j++ {
			initial := []byte{byte(i), byte(j)}
			phrase, err := ToPhrase(initial, English)
			if err != nil {
				t.Fatal(err)
			}
			final, err := FromPhrase(phrase, English)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Compare(initial, final) != 0 {
				t.Error("comparison failed during circular byte check")
			}
		}
	}
	// It takes too long to try all numbers 3 deep, so only a handful are
	// picked. All edge numbers are checked.
	for i := 0; i < 256; i++ {
		for _, j := range []byte{0, 1, 2, 3, 16, 25, 82, 200, 252, 253, 254, 255} {
			for _, k := range []byte{0, 1, 2, 3, 9, 29, 62, 104, 105, 217, 252, 253, 254, 255} {
				initial := []byte{byte(i), j, k}
				phrase, err := ToPhrase(initial, English)
				if err != nil {
					t.Fatal(err)
				}
				final, err := FromPhrase(phrase, English)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Compare(initial, final) != 0 {
					t.Error("comparison failed during circular byte check")
				}
			}
		}
	}

	// Check that all values going from phrase to []byte and back result in the
	// original value, as deep as reasonable.
	for i := 0; i < DictionarySize; i++ {
		initial := Phrase{englishDictionary[i]}
		entropy, err := FromPhrase(initial, English)
		if err != nil {
			t.Fatal(err)
		}
		final, err := ToPhrase(entropy, English)
		if err != nil {
			t.Fatal(err)
		}
		if len(initial) != len(final) {
			t.Fatal("conversion error")
		}
		for i := range initial {
			if initial[i] != final[i] {
				t.Error("conversion error")
			}
		}
	}
	// It takes too long to try all numbers 2 deep for phrases, so the test it
	// not comprehensive. All edge numbers are checked.
	for i := 0; i < DictionarySize; i++ {
		for _, j := range []int{0, 1, 2, 3, 4, 5, 6, 25, 50, 75, 122, 266, 305, 1620, 1621, 1622, 1623, 1623, 1625} {
			initial := Phrase{englishDictionary[i], englishDictionary[j]}
			entropy, err := FromPhrase(initial, English)
			if err != nil {
				t.Fatal(err)
			}
			final, err := ToPhrase(entropy, English)
			if err != nil {
				t.Fatal(err)
			}
			if len(initial) != len(final) {
				t.Fatal("conversion error")
			}
			for i := range initial {
				if initial[i] != final[i] {
					t.Error("conversion error")
				}
			}
		}
	}
	// It takes too long to try all numbers 2 deep for phrases, so the test it
	// not comprehensive. All edge numbers are checked.
	for _, i := range []int{0, 1, 2, 3, 4, 5, 6, 25, 50, 75, 122, 266, 305, 1620, 1621, 1622, 1623, 1623, 1625} {
		for _, j := range []int{0, 1, 2, 3, 4, 5, 6, 25, 50, 75, 122, 266, 305, 1620, 1621, 1622, 1623, 1623, 1625} {
			for _, k := range []int{0, 1, 2, 3, 4, 5, 6, 25, 50, 75, 122, 266, 305, 1620, 1621, 1622, 1623, 1623, 1625} {
				initial := Phrase{englishDictionary[i], englishDictionary[j], englishDictionary[k]}
				entropy, err := FromPhrase(initial, English)
				if err != nil {
					t.Fatal(err)
				}
				final, err := ToPhrase(entropy, English)
				if err != nil {
					t.Fatal(err)
				}
				if len(initial) != len(final) {
					t.Fatal("conversion error")
				}
				for i := range initial {
					if initial[i] != final[i] {
						t.Error("conversion error")
					}
				}
			}
		}
	}
}

// TestNilInputs tries nil and 0 inputs when using the exported functions.
func TestNilInputs(t *testing.T) {
	_, err := ToPhrase(nil, English)
	if err != errEmptyInput {
		t.Error(err)
	}
	_, err = FromPhrase(nil, English)
	if err != errEmptyInput {
		t.Error(err)
	}
	_, err = ToPhrase([]byte{0}, "")
	if err != errUnknownDictionary {
		t.Error(err)
	}
	_, err = FromPhrase(Phrase{"abbey"}, "")
	if err != errUnknownDictionary {
		t.Error(err)
	}

	ps := Phrase{}.String()
	if ps != "" {
		t.Error(ps)
	}
	ps = Phrase{""}.String()
	if ps != "" {
		t.Error(ps)
	}
	ps = Phrase{"a", ""}.String()
	if ps != "a " {
		t.Error(ps)
	}
}

// TestUnrecognizedWord tries to decode a phrase that has an unrecognized word.
func TestUnrecognizedWord(t *testing.T) {
	phrase := Phrase{"zzzzzz"}
	_, err := FromPhrase(phrase, English)
	if err != errUnknownWord {
		t.Error(err)
	}
}

// TestPhraseString calls String() on a Phrase.
func TestPhraseString(t *testing.T) {
	phrase := Phrase{"abc", "def", "g"}
	if phrase.String() != "abc def g" {
		t.Error("Phrase.String() behaving unexpectedly")
	}
}

// TestNormalization tries to decode a non-normalized string.
func TestNormalization(t *testing.T) {
	a := Phrase{"abhärten"}
	b := Phrase{"abh\u00e4rten"}
	c := Phrase{"abha\u0308rten"}
	d := Phrase{"abh\u0061\u0308rten"}

	ba, err := FromPhrase(a, German)
	if err != nil {
		t.Error(err)
	}
	bb, err := FromPhrase(b, German)
	if err != nil {
		t.Error(err)
	}
	bc, err := FromPhrase(c, German)
	if err != nil {
		t.Error(err)
	}
	bd, err := FromPhrase(d, German)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(ba, bb) != 0 {
		t.Error("bad decoding")
	}
	if bytes.Compare(bb, bc) != 0 {
		t.Error("bad decoding")
	}
	if bytes.Compare(bc, bd) != 0 {
		t.Error("bad decoding")
	}
}
