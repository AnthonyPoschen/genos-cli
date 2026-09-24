package creds

import "fmt"

// SystemKeyring stores the token in the OS credential store.
// The Secret Service item uses service "genos" and attribute host set to
// the API origin. It does not use the username attribute.
type SystemKeyring struct{}

func (SystemKeyring) Get(origin string) (string, error) {
	return secretServiceGet(KeyringAttributes(origin))
}

func (SystemKeyring) Set(origin, secret string) error {
	label := fmt.Sprintf("Genos token for %s", origin)
	return secretServiceSet(label, KeyringAttributes(origin), secret)
}
