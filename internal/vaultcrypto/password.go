package vaultcrypto

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strings"
)

// Character sets are listed once so selection is uniform inside the set.
const (
	lettersLower = "abcdefghijkmnopqrstuvwxyz"
	lettersUpper = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digits       = "23456789"
	symbols      = "!@#$%^&*_-+=?"
)

// GeneratePassword uses crypto/rand without modulo bias.
// Ambiguous characters are omitted (0/O, 1/l).
func GeneratePassword(length int, lower, upper, digit, symbol bool) (string, error) {
	if length < 8 || length > 128 {
		return "", fmt.Errorf("%w: length", ErrParams)
	}
	var alphabet string
	if lower {
		alphabet += lettersLower
	}
	if upper {
		alphabet += lettersUpper
	}
	if digit {
		alphabet += digits
	}
	if symbol {
		alphabet += symbols
	}
	if alphabet == "" {
		return "", fmt.Errorf("%w: alphabet", ErrParams)
	}
	out := make([]byte, length)
	for i := range out {
		n, err := cryptoIndex(len(alphabet))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n]
	}
	return string(out), nil
}

func cryptoIndex(n int) (int, error) {
	if n <= 0 {
		return 0, ErrParams
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

// DefaultWordlist is a short original list. Entropy is reported from its
// length so the UI does not imply a 7776-word diceware list.
var DefaultWordlist = []string{
	"amber", "anchor", "apple", "arcade", "aspen", "atlas", "badger", "baker",
	"bamboo", "banjo", "barley", "basil", "beacon", "birch", "bison", "bluff",
	"brass", "brook", "cabin", "cactus", "cedar", "cello", "cinder", "cliff",
	"cloud", "cobalt", "comet", "coral", "crane", "creek", "daisy", "delta",
	"dune", "ember", "falcon", "fern", "flint", "frost", "garden", "glacier",
	"grove", "harbor", "hazel", "heron", "holly", "honey", "island", "ivory",
	"jasper", "juniper", "kettle", "lagoon", "lantern", "lark", "lemon", "lilac",
	"lodge", "maple", "marble", "meadow", "mesa", "mint", "mist", "moss",
	"nectar", "north", "olive", "onyx", "orchid", "otter", "paddle", "pebble",
	"pine", "plaza", "pond", "quartz", "quill", "raven", "ridge", "river",
	"robin", "saffron", "sage", "shore", "sierra", "silver", "sparrow", "spruce",
	"stone", "summit", "thistle", "timber", "tulip", "valley", "violet", "willow",
	"winter", "yarrow",
}

// GeneratePassphrase returns a hyphenated passphrase and its floor entropy in bits.
func GeneratePassphrase(words int, list []string) (string, int, error) {
	if words < 6 || words > 16 {
		return "", 0, fmt.Errorf("%w: word count", ErrParams)
	}
	if len(list) < 64 {
		return "", 0, fmt.Errorf("%w: wordlist", ErrParams)
	}
	parts := make([]string, words)
	for i := range parts {
		n, err := cryptoIndex(len(list))
		if err != nil {
			return "", 0, err
		}
		parts[i] = list[n]
	}
	bits := int(math.Floor(float64(words) * math.Log2(float64(len(list)))))
	return strings.Join(parts, "-"), bits, nil
}
