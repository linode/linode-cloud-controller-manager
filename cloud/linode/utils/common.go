package common

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/linode/linodego"
)

const (
	ProviderIDPrefix          = "linode://"
	DNS1123LabelMaxLength int = 63
)

type InvalidProviderIDError struct {
	Value string
}

func (e InvalidProviderIDError) Error() string {
	return fmt.Sprintf("invalid provider ID %q", e.Value)
}

func IsLinodeProviderID(providerID string) bool {
	return strings.HasPrefix(providerID, ProviderIDPrefix)
}

func ParseProviderID(providerID string) (int, error) {
	if !IsLinodeProviderID(providerID) {
		return 0, InvalidProviderIDError{providerID}
	}
	id, err := strconv.Atoi(strings.TrimPrefix(providerID, ProviderIDPrefix))
	if err != nil {
		return 0, InvalidProviderIDError{providerID}
	}
	return id, nil
}

// IgnoreLinodeAPIError returns the error except matches to status code
func IgnoreLinodeAPIError(err error, code int) error {
	apiErr := linodego.Error{Code: code}
	if apiErr.Is(err) {
		err = nil
	}

	return err
}

func IsPrivate(ip *net.IP, linodeExternalNetwork *net.IPNet) bool {
	if linodeExternalNetwork == nil {
		return ip.IsPrivate()
	}

	return ip.IsPrivate() && !linodeExternalNetwork.Contains(*ip)
}
