package auth

import "errors"

// Startup-fatal validation errors. Surfacing them as sentinels lets the caller
// (app startup) terminate with a precise message instead of a generic one.
func errInvalidPublicURL(v string) error {
	return errors.New("ZLT_PUBLIC_URL must be a valid https root URL without userinfo/path/query: " + v)
}

func errInvalidProxy(v string) error {
	return errors.New("invalid trusted proxy entry (need IP or CIDR): " + v)
}
