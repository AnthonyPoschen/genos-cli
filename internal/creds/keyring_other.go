//go:build !linux

package creds

import "errors"

func secretServiceGet(map[string]string) (string, error) {
	return "", errors.New("secret service is only available on linux")
}

func secretServiceSet(string, map[string]string, string) error {
	return errors.New("secret service is only available on linux")
}
