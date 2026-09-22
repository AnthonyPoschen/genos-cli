package creds

import "github.com/zalando/go-keyring"

// SystemKeyring stores the token with service "genos" and the username
// attribute set to the API origin.
type SystemKeyring struct{}

func (SystemKeyring) Get(origin string) (string, error) {
	service, account := KeyringItem(origin)
	return keyring.Get(service, account)
}

func (SystemKeyring) Set(origin, secret string) error {
	service, account := KeyringItem(origin)
	return keyring.Set(service, account, secret)
}
