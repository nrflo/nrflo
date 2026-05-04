package scaffold

import (
	"fmt"
	"regexp"
)

var tokenRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// Substitute replaces all {{KEY}} tokens in b using vars.
// Returns an error if b contains a token whose key is not in vars.
func Substitute(b []byte, vars map[string]string) ([]byte, error) {
	var subErr error
	result := tokenRe.ReplaceAllFunc(b, func(match []byte) []byte {
		if subErr != nil {
			return match
		}
		key := string(tokenRe.FindSubmatch(match)[1])
		val, ok := vars[key]
		if !ok {
			subErr = fmt.Errorf("unknown template token {{%s}}", key)
			return match
		}
		return []byte(val)
	})
	if subErr != nil {
		return nil, subErr
	}
	return result, nil
}
