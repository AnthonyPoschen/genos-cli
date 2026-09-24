//go:build linux

package creds

import (
	"errors"

	dbus "github.com/godbus/dbus/v5"
	ss "github.com/zalando/go-keyring/secret_service"
)

func secretServiceGet(attributes map[string]string) (string, error) {
	svc, err := ss.NewSecretService()
	if err != nil {
		return "", err
	}
	collection := svc.GetLoginCollection()
	if err := svc.Unlock(collection.Path()); err != nil {
		return "", err
	}
	results, err := svc.SearchItems(collection, attributes)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", errors.New("secret not found")
	}
	session, err := svc.OpenSession()
	if err != nil {
		return "", err
	}
	defer svc.Close(session)
	if err := svc.Unlock(results[0]); err != nil {
		return "", err
	}
	secret, err := svc.GetSecret(results[0], session.Path())
	if err != nil {
		return "", err
	}
	return string(secret.Value), nil
}

func secretServiceSet(label string, attributes map[string]string, secret string) error {
	svc, err := ss.NewSecretService()
	if err != nil {
		return err
	}
	session, err := svc.OpenSession()
	if err != nil {
		return err
	}
	defer svc.Close(session)
	collection := svc.GetLoginCollection()
	if err := svc.Unlock(collection.Path()); err != nil {
		return err
	}
	return svc.CreateItem(collection, label, attributes, ss.NewSecret(dbus.ObjectPath(session.Path()), secret))
}
