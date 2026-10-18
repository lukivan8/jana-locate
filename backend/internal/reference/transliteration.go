package reference

import "strings"

// LatinKey supplies a deterministic component alias, never an address answer.
// Collisions retain every reference identity in the ordinary alias index.
func LatinKey(value string) string {
	var out strings.Builder
	for _, c := range Canon(value) {
		if v, ok := latinLetters[c]; ok {
			out.WriteString(v)
		} else {
			out.WriteRune(c)
		}
	}
	return out.String()
}

var latinLetters = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	'ә': "a", 'ғ': "gh", 'қ': "q", 'ң': "ng", 'ө': "o", 'ұ': "u", 'ү': "u", 'һ': "h", 'і': "i",
}
