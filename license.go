package main

import (
	_ "embed"
	"fmt"
)

//go:embed LICENSE
var licenseText string

//go:embed THIRD_PARTY_LICENSES.txt
var thirdPartyLicenses string

func licenseCmd() error {
	fmt.Println(licenseText)
	fmt.Println(thirdPartyLicenses)
	return nil
}
