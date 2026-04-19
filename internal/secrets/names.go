package secrets

import "strings"

func NormalizeKeychainName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "-")
	return name
}
