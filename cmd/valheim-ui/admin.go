package main

import "errors"

// runAdmin provides break-glass user administration:
//
//	valheim-ui admin create-user --username U --role admin [--password P]
//	valheim-ui admin reset-password --username U [--password P]
//	valheim-ui admin list-users
//
// WP-01 implements it on top of internal/auth.
func runAdmin(args []string) error {
	_ = args
	return errors.New("admin: not implemented (WP-01)")
}
