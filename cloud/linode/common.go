package linode

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/linode/linodego"
)

const (
	providerIDPrefix          = "linode://"
	DNS1123LabelMaxLength int = 63
)

type invalidProviderIDError struct {
	value string
}

func (e invalidProviderIDError) Error() string {
	return fmt.Sprintf("invalid provider ID %q", e.value)
}

func isLinodeProviderID(providerID string) bool {
	return strings.HasPrefix(providerID, providerIDPrefix)
}

func parseProviderID(providerID string) (int, error) {
	if !isLinodeProviderID(providerID) {
		return 0, invalidProviderIDError{providerID}
	}
	id, err := strconv.Atoi(strings.TrimPrefix(providerID, providerIDPrefix))
	if err != nil {
		return 0, invalidProviderIDError{providerID}
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

func isPrivate(ip *net.IP) bool {
	if Options.LinodeExternalNetwork == nil {
		return ip.IsPrivate()
	}

	return ip.IsPrivate() && !Options.LinodeExternalNetwork.Contains(*ip)
}
