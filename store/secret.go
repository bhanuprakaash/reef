package store

import "regexp"

// SecretMarker replaces each secret RemoveSecrets finds.
const SecretMarker = "[secret removed]"

var _secrets = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[A-Za-z0-9._~+/=-]+`), "${1}" + SecretMarker},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`), SecretMarker},
	{regexp.MustCompile(`(?i)(\b(?:postgres(?:ql)?|mongodb(?:\+srv)?)://[^:/\s@]+:)[^@\s/]+@`), "${1}" + SecretMarker + "@"},
	{regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{16,}|ghp_[A-Za-z0-9]{20,}|glpat-[A-Za-z0-9_-]{16,}|xox[abprs]-[A-Za-z0-9-]{10,})`), SecretMarker},
}

// RemoveSecrets replaces bearer tokens, JWTs, passwords in database URLs and known API keys
// in text with SecretMarker, and reports whether it replaced any.
func RemoveSecrets(text string) (string, bool) {
	out := text
	for _, s := range _secrets {
		out = s.re.ReplaceAllString(out, s.repl)
	}
	return out, out != text
}
