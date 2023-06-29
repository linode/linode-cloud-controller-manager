package linode

import (
	"fmt"
	"strconv"
	"strings"
)

const providerIDPrefix = "linode://"

type invalidProviderIDError struct {
	value string
}

func (e invalidProviderIDError) Error() string {
	return fmt.Sprintf("invalid provider ID %q", e.value)
}

func parseProviderID(providerID string) (int, error) {
	if !strings.HasPrefix(providerID, providerIDPrefix) {
		return 0, invalidProviderIDError{providerID}
	}
	id, err := strconv.Atoi(strings.TrimPrefix(providerID, providerIDPrefix))
	if err != nil {
		return 0, invalidProviderIDError{providerID}
	}
	return id, nil
}
