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
