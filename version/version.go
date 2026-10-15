package version

// version is overridden by build time flags
var version string

func GetVersion() string {
	if version == "" {
		return "dev"
	}

	return version
}
