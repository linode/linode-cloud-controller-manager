package linode

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/linode/linodego"
)

const (
	providerIDPrefix = "linode://"
	// context timeout for API requests
	requestTimeout = 120 * time.Second
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
